# Метрики (Prometheus)

Бэкенд отдаёт метрики в формате Prometheus на **отдельном порту** — по умолчанию
`9090`, путь `/metrics`.

Отдельный слушатель, а не маршрут на API-сервере, выбран намеренно: и ingress
чарта, и nginx фронтенда проксируют на бэкенд только префикс `/api`, поэтому
`/metrics` недоступен с публичного хоста, но его по-прежнему видит скрейпер
внутри кластера. Эндпоинт **не аутентифицирован**, как и остальной API, и уже по
одним именам метрик видно количество планов, выбранную модель и накопленную
стоимость LLM — всё, что может достучаться до пода, это прочитает.

## Настройка

`config.yaml`:

```yaml
metrics:
  enabled: true
  host: "0.0.0.0"
  port: 9090
  path: /metrics
  refreshSeconds: 30
```

Переменные окружения перекрывают YAML: `METRICS_ENABLED`, `METRICS_HOST`,
`METRICS_PORT`, `METRICS_PATH`, `METRICS_REFRESH_SECONDS`. Так и должно быть —
Dockerfile и Helm-деплой передают `METRICS_*` через env, и `config.yaml`,
вшитый в образ или оставшийся на data-волюме, не должен их переопределять.

Порт не может совпадать с `SERVER_PORT`: бэкенд откажется стартовать.

### Helm

```yaml
backend:
  metrics:
    enabled: true
    port: 9090
    path: /metrics
    service:
      enabled: true          # добавить порт metrics в Service
    serviceMonitor:
      enabled: true          # нужен prometheus-operator
      interval: 30s
```

`ServiceMonitor` рендерится только если в кластере есть API
`monitoring.coreos.com/v1`, поэтому на кластере без prometheus-operator шаблон
просто ничего не создаёт. Альтернатива — аннотации на поде:

```yaml
backend:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9090"
    prometheus.io/path: "/metrics"
```

## Что отдаётся

### Go и процесс

Стандартные коллекторы `client_golang`: `go_*` (включая GC и задержки
планировщика) и `process_*` (CPU, RSS, файловые дескрипторы,
`process_start_time_seconds`). Плюс `nib_build_info{version,go_version}` и
`nib_start_time_seconds`.

### HTTP

| Метрика | Тип | Метки |
|---|---|---|
| `nib_http_requests_total` | counter | `method`, `route`, `code` |
| `nib_http_request_duration_seconds` | histogram | `method`, `route` |
| `nib_http_response_bytes_total` | counter | `method`, `route` |
| `nib_http_requests_in_flight` | gauge | — |

`route` — это шаблон маршрута chi (`/api/v1/dialogs/{id}/messages`), а не путь
запроса: почти во всех путях есть UUID, и `r.URL.Path` дал бы по серии на диалог.
Запрос, не попавший ни в один маршрут, получает метку `unmatched`, а CORS-preflight —
`preflight` (`corsMiddleware` отвечает на `OPTIONS` до маршрутизации).

Бакеты доходят до 1800 секунд: маршруты агента (`/chat`, `/messages`,
`/messages/retry`, `/tool-results`) работают под 30-минутным write deadline, и с
бакетами по умолчанию (до 10 с) все они оказались бы в `+Inf`.

Учтите, что `/api/v1/dialogs/{id}/events` — это SSE-стрим: его длительность равна
времени жизни подписки, а объём ответа — всем переданным событиям. На дашбордах
этот маршрут стоит исключать из перцентилей латентности.

### Планы

| Метрика | Тип | Метки |
|---|---|---|
| `nib_plans` | gauge | `status` |
| `nib_plans_refresh_duration_seconds` | histogram | — |
| `nib_plans_refresh_errors_total` | counter | — |
| `nib_plans_refresh_timestamp_seconds` | gauge | — |
| `nib_plan_status_transitions_total` | counter | `from`, `to` |

Статусы: `draft`, `scheduled`, `in_progress`, `done`, `reopened`, `rolled_back`, `cancelled`.
Публикуются все шесть, в том числе нулевые.

Статус плана лежит не в Postgres, а в `data/plan_state/{dialogID}.json` (и по
умолчанию равен `draft`, если файла нет), поэтому подсчёт — это один запрос плюс
одно чтение файла на каждый план. Он выполняется фоновым обновлением раз в
`refreshSeconds`, а не на каждый скрейп. Если планов станет очень много, следите
за `nib_plans_refresh_duration_seconds`; правильное решение в этом случае —
колонка статуса в `chat_dialogs`, но это отдельная задача.

### База данных

`nib_db_up` (0/1) и `nib_db_ping_duration_seconds` — реальный признак живости,
которого не даёт `GET /api/v1/health`: тот отвечает `ok` безусловно и до базы не
дотрагивается.

Пул pgx: `nib_db_pool_connections{state}`, `nib_db_pool_max_connections`,
`nib_db_pool_acquires_total`, `nib_db_pool_empty_acquires_total`,
`nib_db_pool_canceled_acquires_total`, `nib_db_pool_new_connections_total`,
`nib_db_pool_destroys_total{reason}`, `nib_db_pool_acquire_wait_seconds_total`,
`nib_db_pool_empty_acquire_wait_seconds_total`.

### Агент и LLM

| Метрика | Тип | Метки |
|---|---|---|
| `nib_agent_turns_total` | counter | `mode`, `outcome` |
| `nib_agent_turn_duration_seconds` | histogram | `mode` |
| `nib_agent_rounds` | histogram | `mode` |
| `nib_agent_tool_failures_total` | counter | `mode` |
| `nib_agent_turns_in_flight` | gauge | `mode` |
| `nib_agent_tool_calls_total` | counter | `source`, `outcome` |
| `nib_agent_local_tool_calls_total` | counter | `tool`, `outcome` |
| `nib_agent_tool_call_duration_seconds` | histogram | `source` |
| `nib_llm_requests_total` | counter | `operation`, `model`, `outcome` |
| `nib_llm_request_duration_seconds` | histogram | `operation`, `model` |
| `nib_llm_tool_calls_returned_total` | counter | `model` |
| `nib_llm_embedding_tokens_total` | counter | `model` |
| `nib_mcp_tool_discovery_duration_seconds` | histogram | — |
| `nib_mcp_tool_discovery_errors_total` | counter | `server` |
| `nib_mcp_tools_discovered` | gauge | `server` |

`outcome` у тёрна: `success`, `error`, `max_iterations`, `tool_failures`,
`awaiting_input`. `nib_agent_rounds` показывает, сколько раундов из бюджета
`agent.maxIterations` тёрн успел сжечь.

Имена инструментов есть только в `nib_agent_local_tool_calls_total` и только для
встроенных Go-инструментов: имена MCP приходят из конфигурации оператора, а
выдуманное имя — от модели, так что ни то ни другое не должно попадать в метку.
`source` — `local`, `mcp` или `unknown`.

Токены промпта и ответа в Prometheus по-прежнему не попадают, но с версии,
добавившей статистику, они больше не теряются: `llm.AssistantMessage` несёт
`Usage`, и каждый вызов пишется строкой в таблицу `llm_usage` (см.
[статистика](#статистика)). Счётчик `nib_llm_embedding_tokens_total` остаётся
как был.

### Выполнение

| Метрика | Тип | Метки |
|---|---|---|
| `nib_execution_lease_held` | gauge | — |
| `nib_execution_lease_acquisitions_total` | counter | `kind` |
| `nib_execution_lease_rejections_total` | counter | `kind` |
| `nib_execution_lease_expirations_total` | counter | — |
| `nib_execution_lease_hold_seconds` | histogram | `kind` |
| `nib_execution_force_stops_total` | counter | `kind`, `outcome` |
| `nib_action_exec_runs_started_total` | counter | `kind` |
| `nib_action_exec_runs_finished_total` | counter | `status` |
| `nib_action_exec_run_duration_seconds` | histogram | `status` |
| `nib_action_exec_active` | gauge | — |
| `nib_plan_fanout_runs_started_total` | counter | — |
| `nib_plan_fanout_runs_finished_total` | counter | `status` |
| `nib_plan_fanout_run_duration_seconds` | histogram | `status` |
| `nib_plan_fanout_stages_total` | counter | `kind`, `status` |
| `nib_plan_fanout_blockers_total` | counter | — |
| `nib_plan_fanout_rejections_total` | counter | `reason` |
| `nib_plan_fanout_active` | gauge | — |
| `nib_stuck_runs_reconciled_total` | counter | `kind` |

`nib_execution_lease_rejections_total` — прямая мера того, как часто политика
«одно выполнение за раз» отказывает операторам: второй запрос не встаёт в
очередь, а отклоняется.

`nib_stuck_runs_reconciled_total` больше нуля после старта означает, что
предыдущий процесс умер в середине выполнения.

### Контейнеры агента

| Метрика | Тип | Метки |
|---|---|---|
| `nib_executor_runs_total` | counter | `entrypoint`, `type`, `platform`, `outcome` |
| `nib_executor_run_duration_seconds` | histogram | `entrypoint` |
| `nib_executor_stops_total` | counter | `outcome` |
| `nib_agent_runner_webhooks_total` | counter | `outcome` |
| `nib_agent_runner_results_total` | counter | `status`, `pushed` |
| `nib_agent_runner_run_duration_seconds` | histogram | — |
| `nib_agent_runner_turns` | histogram | — |
| `nib_agent_runner_cost_usd_total` | counter | — |

`nib_executor_run_duration_seconds` — это время **запуска** контейнера, а не его
работы: `RunAction` возвращается сразу после создания job, а о результате
контейнер сообщает вебхуком. Длительность самой работы — в
`nib_agent_runner_run_duration_seconds`.

`status` из вебхука приводится к `success|failed|timeout|error|other`: он
приходит из контейнера по сети, и без этого любая строка стала бы новой серией.

### SSE

`nib_sse_subscribers`, `nib_sse_events_published_total{kind}`,
`nib_sse_events_dropped_total{kind}`.

Потерянное событие означает, что UI разошёлся с состоянием сервера: у брокера
буфер на 32 события, и медленному подписчику события просто выбрасываются.
Раньше об этом говорила только строчка в логе.

## Статистика

Метрики Prometheus отвечают на вопрос «что происходит сейчас» и живут ровно
столько, сколько настроено в самом Prometheus. Для вопросов вида «сколько
токенов и денег ушло на эту задачу за месяц» с версии, добавившей страницу
Statistics, есть три таблицы в Postgres, которые хранятся бессрочно:

| Таблица | Что в ней |
|---|---|
| `llm_usage` | одна строка на вызов LLM: диалог, план, режим, модель, токены (промпт, кэш, ответ, reasoning), стоимость, длительность, исход |
| `agent_run_usage` | одна строка на завершённый контейнер агента, из вебхука |
| `plan_status_transitions` | журнал переходов статуса плана |

Поверх них работает `GET /api/v1/stats?from=&to=&bucket=`, который и рисует
страницу Statistics в веб-интерфейсе. `bucket` — один из `hour`, `day`, `week`,
`month`; диапазон ограничен 400 корзинами, иначе запрос отклоняется с 400.

Два места, где легко ошибиться при чтении этих данных:

- `cost_usd` может быть `NULL`. nib не считает стоимость сам и хранит только то,
  что вернул провайдер: OpenRouter отдаёт `usage.cost`, платформа OpenAI — нет.
  `NULL` означает «неизвестно», а не «бесплатно», и интерфейс показывает там
  прочерк, а не `$0.00`.
- `cached_prompt_tokens` — это подмножество `prompt_tokens`, а
  `reasoning_tokens` — подмножество `completion_tokens`. Складывать все четыре
  нельзя: получится двойной счёт.

Строки намеренно переживают удаление диалога — внешнего ключа на `chat_dialogs`
нет, иначе `ON DELETE CASCADE` стирал бы историю расходов вместе с диалогом.
Поэтому любое соединение с `chat_dialogs` должно быть `LEFT JOIN`.

История статусов начинает накапливаться только с момента выката: у планов,
существовавших раньше, переходов в журнале нет. Текущий срез по статусам при
этом полный — он по-прежнему считается по файлам `plan_state`.

## Полезные запросы

| Что проверяем | Запрос |
|---|---|
| API отдаёт ошибки | `sum(rate(nib_http_requests_total{code=~"5.."}[5m])) > 0` |
| Латентность API | `histogram_quantile(0.95, sum by (le,route) (rate(nib_http_request_duration_seconds_bucket{route!="/api/v1/dialogs/{id}/events"}[5m])))` |
| База недоступна | `nib_db_up == 0` |
| Пул на исходе | `nib_db_pool_connections{state="acquired"} / nib_db_pool_max_connections > 0.9` |
| Провайдер LLM деградирует | `sum(rate(nib_llm_requests_total{outcome="error"}[5m])) / sum(rate(nib_llm_requests_total[5m])) > 0.1` |
| Агент упирается в лимит раундов | `increase(nib_agent_turns_total{outcome="max_iterations"}[30m]) > 0` |
| Сломанный MCP-сервер | `topk(5, sum by (server) (rate(nib_mcp_tool_discovery_errors_total[15m])))` |
| Инструменты падают | `sum by (source) (rate(nib_agent_tool_calls_total{outcome="error"}[15m]))` |
| Операторов блокирует lease | `increase(nib_execution_lease_rejections_total[15m]) > 3` |
| UI разошёлся с сервером | `increase(nib_sse_events_dropped_total[10m]) > 0` |
| Процесс падал в середине прогона | `increase(nib_stuck_runs_reconciled_total[1h]) > 0` |
| Гейджи планов устарели | `time() - nib_plans_refresh_timestamp_seconds > 300` |
| Стоимость контейнерных агентов за сутки | `increase(nib_agent_runner_cost_usd_total[24h])` |
| Бэкенд недоступен | `up{job="nib-backend"} == 0` |

## Замечание про масштабирование

Lease выполнения, SSE-брокер, все локальные блокировки и фоновое обновление
планов — процессные, а чарт фиксирует `backend.replicaCount: 1`. Поэтому
`nib_execution_lease_held`, `nib_sse_subscribers` и `nib_plans` сейчас однозначны,
но при нескольких репликах станут метриками отдельного пода — суммировать их по
репликам нельзя.
