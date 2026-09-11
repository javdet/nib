import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import {
	Send,
	Square,
	Loader2,
	User,
	Bot,
	Paperclip,
	X,
	FileText,
	RotateCcw,
	AlertTriangle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { IconButton } from '@/components/ui/icon-button'
import { fieldClasses } from '@/components/ui/input'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn, findLastIndex } from '@/lib/utils'
import { extractErrorMessage, isTransportError } from '@/lib/api-client'
import { useMode } from '@/features/modes/mode-context'
import { useDialog } from '@/features/dialogs/dialog-context'
import {
	createDialog,
	getDialogMessages,
	sendDialogMessage,
	retryLastResponse,
	submitToolResults,
	uploadAttachment,
	deleteAttachment,
	type Attachment,
	type DialogChatResponse,
	type DialogMessage,
	type Question,
	type TableData,
} from '@/features/dialogs/api/dialogs'
import { AskQuestionMessage } from '@/features/dialogs/components/ask-question-message'
import { AnsweredQuestionsMessage } from '@/features/dialogs/components/answered-questions-message'
import { TableMessage } from '@/features/dialogs/components/table-message'
import { ChatHistoryMenu } from '@/features/dialogs/components/chat-history-menu'
import { CopyButton } from '@/components/copy-button'
import { MarkdownMessage } from '@/components/markdown-message'
import { findPendingAskQuestion } from '@/features/dialogs/lib/pending-question'
import {
	parseAnsweredQuestions,
	type AnsweredQuestion,
} from '@/features/dialogs/lib/answered-questions'
import { parseCreateTableFromToolCalls } from '@/features/dialogs/lib/parse-table'
import {
	hasLastUserMessage,
	hasToolResult,
	isTurnComplete,
} from '@/features/dialogs/lib/turn-state'
import { useThinkingPhrase } from '@/features/dialogs/hooks/use-thinking-phrase'
import { useToolActivity } from '@/features/dialogs/hooks/use-tool-activity'
import { useSkills } from '@/features/skills/hooks/use-skills'
import { useAutosizeTextarea } from '@/hooks/use-autosize-textarea'
import {
	SkillSuggestMenu,
	SKILL_LISTBOX_ID,
	skillOptionId,
} from '@/features/skills/components/skill-suggest-menu'
import {
	applySkillSelection,
	matchSkills,
	shouldRearmSkillMenu,
	skillMenuQuery,
} from '@/features/skills/lib/skill-suggest'

const RECONNECT_POLL_MS = 3000
const RECONNECT_MAX_MS = 30 * 60 * 1000

interface Message {
	role: 'user' | 'assistant'
	content: string
	table?: TableData
	answers?: AnsweredQuestion[]
	attachments?: Attachment[]
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
	return new Promise((resolve) => {
		if (signal.aborted) {
			resolve()
			return
		}
		const timer = setTimeout(() => {
			signal.removeEventListener('abort', onAbort)
			resolve()
		}, ms)
		function onAbort() {
			clearTimeout(timer)
			resolve()
		}
		signal.addEventListener('abort', onAbort, { once: true })
	})
}

function AnimatedDots() {
	return (
		<span className="inline-flex gap-0.5" aria-hidden="true">
			{[0, 1, 2].map((i) => (
				<span
					key={i}
					className="inline-block h-1 w-1 animate-pulse rounded-full bg-current"
					style={{ animationDelay: `${i * 200}ms` }}
				/>
			))}
		</span>
	)
}

/**
 * Reports whether a bubble is a real user turn.
 *
 * The answered-questions bubble renders user-side but is stored as a `tool`
 * row, and `RetryLastResponse` truncates from the last `user` row — so it must
 * not count when working out where a retry lands.
 */
function isUserTurn(m: Message): boolean {
	return m.role === 'user' && !m.answers
}

function toUiMessages(msgs: DialogMessage[]): Message[] {
	const out: Message[] = []
	for (const m of msgs) {
		// The answered set is stored only as an ask_question tool result, so it
		// has to be rendered from here or it disappears on every reload.
		if (m.role === 'tool' && m.name === 'ask_question') {
			const answers = parseAnsweredQuestions(m.content)
			if (answers) {
				out.push({ role: 'user', content: '', answers })
			}
			continue
		}
		if (m.role === 'user') {
			out.push({
				role: 'user',
				content: m.content,
				attachments: m.attachments,
			})
			continue
		}
		if (m.role === 'assistant') {
			if (m.content.trim().length > 0) {
				out.push({
					role: 'assistant',
					content: m.content,
					attachments: m.attachments,
				})
			}
			for (const table of parseCreateTableFromToolCalls(m.toolCalls)) {
				out.push({
					role: 'assistant',
					content: '',
					table,
				})
			}
		}
	}
	return out
}

export function ChatPanel() {
	const { selectedMode } = useMode()
	const {
		activeDialogId,
		setActiveDialogId,
		bumpDialogsVersion,
		bumpActionPlanVersion,
		pendingMessages,
		consumePendingMessage,
	} = useDialog()
	const [draft, setDraft] = useState('')
	const [messages, setMessages] = useState<Message[]>([])
	const [loading, setLoading] = useState(false)
	const [messagesLoading, setMessagesLoading] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [pendingQuestions, setPendingQuestions] = useState<Question[]>([])
	const [pendingToolCallId, setPendingToolCallId] = useState<string | null>(
		null,
	)
	const [submittingAnswers, setSubmittingAnswers] = useState(false)
	const [reconnecting, setReconnecting] = useState(false)
	const [pendingAttachments, setPendingAttachments] = useState<Attachment[]>([])
	const [uploading, setUploading] = useState(false)
	const [activeSkillIndex, setActiveSkillIndex] = useState(0)
	const [skillMenuDismissed, setSkillMenuDismissed] = useState(false)
	const scrollRef = useRef<HTMLDivElement>(null)
	const abortRef = useRef<AbortController | null>(null)
	const dialogIdRef = useRef<string | null>(null)
	const loadedDialogIdRef = useRef<string | null>(null)
	// The dialog this panel created for a turn it is already running. Adopting
	// its id is not a context switch, so the reset below has to skip it.
	const selfCreatedDialogIdRef = useRef<string | null>(null)
	const fileInputRef = useRef<HTMLInputElement>(null)
	const textareaRef = useRef<HTMLTextAreaElement>(null)
	const panelRef = useRef<HTMLDivElement>(null)
	const composerFrameRef = useRef<HTMLDivElement>(null)

	// The composer grows with the draft but never past half the chat panel.
	useAutosizeTextarea({
		textareaRef,
		containerRef: panelRef,
		frameRef: composerFrameRef,
		value: draft,
	})

	const hasPendingQuestions = pendingQuestions.length > 0
	const inputDisabled = loading || messagesLoading || hasPendingQuestions

	const { skills, reload: reloadSkills, isStale: areSkillsStale } = useSkills()
	// Open/closed and the query are derived from the draft, so deleting the slash,
	// typing a space or sending all close the menu without any bookkeeping.
	const skillQuery = skillMenuQuery(draft, skillMenuDismissed)
	const skillSuggestions = useMemo(
		() => (skillQuery === null ? [] : matchSkills(skills, skillQuery)),
		[skills, skillQuery],
	)
	const isSkillMenuOpen = !inputDisabled && skillSuggestions.length > 0

	const isSkillMenuArmed = skillQuery !== null
	useEffect(() => {
		if (isSkillMenuArmed && areSkillsStale()) {
			void reloadSkills()
		}
	}, [isSkillMenuArmed, areSkillsStale, reloadSkills])

	useEffect(() => {
		scrollRef.current?.scrollTo({
			top: scrollRef.current.scrollHeight,
			behavior: 'smooth',
		})
	}, [messages, loading, pendingQuestions])

	useEffect(() => {
		dialogIdRef.current = activeDialogId
	}, [activeDialogId])

	useEffect(() => {
		// A first message in an empty panel creates the dialog mid-send, so this
		// effect fires on an id that already owns the optimistic user bubble and
		// the turn in flight. Wiping them there blanks the conversation until the
		// answer lands. The ref is cleared only when some other dialog becomes
		// active, so a re-run under StrictMode still skips.
		if (activeDialogId && selfCreatedDialogIdRef.current === activeDialogId) {
			loadedDialogIdRef.current = activeDialogId
			return
		}
		selfCreatedDialogIdRef.current = null

		loadedDialogIdRef.current = null
		setMessages([])
		setError(null)
		setPendingQuestions([])
		setPendingToolCallId(null)
		setPendingAttachments([])
		setLoading(false)
		setSubmittingAnswers(false)
		setReconnecting(false)
		setSkillMenuDismissed(false)
		setActiveSkillIndex(0)

		if (!activeDialogId) {
			setMessagesLoading(false)
			return
		}

		let cancelled = false
		setMessagesLoading(true)

		getDialogMessages(activeDialogId)
			.then((msgs) => {
				if (!cancelled) {
					setMessages(toUiMessages(msgs))
					const pending = findPendingAskQuestion(msgs)
					if (pending) {
						setPendingQuestions(pending.questions)
						setPendingToolCallId(pending.toolCallId)
					}
					loadedDialogIdRef.current = activeDialogId
				}
			})
			.catch((err) => {
				if (!cancelled) {
					setError(extractErrorMessage(err))
					setMessages([])
				}
			})
			.finally(() => {
				if (!cancelled) {
					setMessagesLoading(false)
				}
			})

		return () => {
			cancelled = true
		}
	}, [activeDialogId])

	const applyChatResponse = useCallback((resp: DialogChatResponse): boolean => {
		if (resp.actionPlanUpdated) {
			bumpActionPlanVersion()
		}

		if (resp.status === 'awaiting_input' && resp.questions?.length) {
			setPendingToolCallId(resp.toolCallId ?? null)
			setPendingQuestions(resp.questions)
			setLoading(false)
			return true
		}

		setPendingQuestions([])
		setPendingToolCallId(null)
		return false
	}, [bumpActionPlanVersion])

	const applyServerMessages = useCallback((msgs: DialogMessage[]) => {
		setMessages(toUiMessages(msgs))
		const pending = findPendingAskQuestion(msgs)
		if (pending) {
			setPendingQuestions(pending.questions)
			setPendingToolCallId(pending.toolCallId)
		} else {
			setPendingQuestions([])
			setPendingToolCallId(null)
		}
	}, [])

	const reloadMessages = useCallback(
		async (dialogId: string) => {
			const msgs = await getDialogMessages(dialogId)
			if (dialogIdRef.current !== dialogId) {
				return
			}
			applyServerMessages(msgs)
		},
		[applyServerMessages],
	)

	const handleAgentResult = useCallback(() => {
		if (!activeDialogId) {
			return
		}
		void reloadMessages(activeDialogId)
	}, [activeDialogId, reloadMessages])

	// An action sub-agent finishes on its own schedule, so its result can land
	// while the operator is mid-turn here. Reloading then would replace the
	// optimistic user bubble and re-show a question they have already answered,
	// so a busy panel defers the refresh to the end of the turn instead.
	const [pendingActionRefresh, setPendingActionRefresh] = useState(false)

	const handleActionResult = useCallback(() => {
		if (!activeDialogId) {
			return
		}
		if (loading || submittingAnswers) {
			setPendingActionRefresh(true)
			return
		}
		void reloadMessages(activeDialogId)
	}, [activeDialogId, loading, submittingAnswers, reloadMessages])

	useEffect(() => {
		if (!pendingActionRefresh || loading || submittingAnswers) {
			return
		}
		if (!activeDialogId) {
			setPendingActionRefresh(false)
			return
		}
		setPendingActionRefresh(false)
		void reloadMessages(activeDialogId)
	}, [
		pendingActionRefresh,
		loading,
		submittingAnswers,
		activeDialogId,
		reloadMessages,
	])

	// A sub-agent's result links back to the dialog it ran in. Opening it is a
	// context switch rather than navigation, so the link is handled here.
	const handleOpenDialogLink = useCallback(
		(dialogId: string) => {
			setActiveDialogId(dialogId)
		},
		[setActiveDialogId],
	)

	const callingTool = useToolActivity(
		activeDialogId,
		handleAgentResult,
		handleActionResult,
	)
	const thinkingPhrase = useThinkingPhrase(loading)

	/**
	 * A dropped connection does not stop the agent: the backend keeps running the
	 * turn and persists every round. Poll the stored transcript until the turn
	 * ends so the answer shows up without a page reload. `turnStarted` tells a
	 * turn that is still running from one the backend never received.
	 */
	const awaitDroppedTurn = useCallback(
		async (
			dialogId: string,
			signal: AbortSignal,
			turnStarted: (msgs: DialogMessage[]) => boolean,
		): Promise<boolean> => {
			const deadline = Date.now() + RECONNECT_MAX_MS
			while (Date.now() < deadline) {
				await delay(RECONNECT_POLL_MS, signal)
				if (signal.aborted || dialogIdRef.current !== dialogId) {
					return false
				}
				let msgs: DialogMessage[]
				try {
					msgs = await getDialogMessages(dialogId)
				} catch {
					continue
				}
				if (dialogIdRef.current !== dialogId) {
					return false
				}
				if (!turnStarted(msgs)) {
					return false
				}
				if (isTurnComplete(msgs)) {
					applyServerMessages(msgs)
					return true
				}
			}
			return false
		},
		[applyServerMessages],
	)

	const recoverDroppedTurn = useCallback(
		async (
			err: unknown,
			dialogId: string,
			signal: AbortSignal,
			turnStarted: (msgs: DialogMessage[]) => boolean = () => true,
		): Promise<boolean> => {
			if (!isTransportError(err)) {
				return false
			}
			setReconnecting(true)
			try {
				const recovered = await awaitDroppedTurn(
					dialogId,
					signal,
					turnStarted,
				)
				if (recovered) {
					bumpDialogsVersion()
					bumpActionPlanVersion()
				}
				return recovered
			} finally {
				setReconnecting(false)
			}
		},
		[awaitDroppedTurn, bumpDialogsVersion, bumpActionPlanVersion],
	)

	const ensureDialog = useCallback(async (): Promise<string> => {
		let dialogId = activeDialogId ?? dialogIdRef.current
		if (!dialogId) {
			const created = await createDialog({ mode: selectedMode })
			dialogId = created.id
			selfCreatedDialogIdRef.current = created.id
			setActiveDialogId(created.id)
			bumpDialogsVersion()
		}
		dialogIdRef.current = dialogId
		return dialogId
	}, [activeDialogId, selectedMode, setActiveDialogId, bumpDialogsVersion])

	const sendMessage = useCallback(
		async (text: string, sentAttachments: Attachment[] = []) => {
			if (
				(!text && sentAttachments.length === 0) ||
				loading ||
				messagesLoading ||
				hasPendingQuestions
			) {
				return
			}

			setError(null)
			const attachmentIds = sentAttachments.map((a) => a.id)
			setMessages((prev) => [
				...prev,
				{
					role: 'user',
					content: text,
					attachments:
						sentAttachments.length > 0 ? sentAttachments : undefined,
				},
			])
			setLoading(true)

			const controller = new AbortController()
			abortRef.current = controller
			let targetDialogId: string | null = null

			try {
				targetDialogId = await ensureDialog()

				const resp = await sendDialogMessage(
					targetDialogId,
					text,
					attachmentIds,
					controller.signal,
				)
				bumpDialogsVersion()
				if (dialogIdRef.current !== targetDialogId) {
					setLoading(false)
					return
				}
				const awaitingInput = applyChatResponse(resp)
				if (!awaitingInput) {
					await reloadMessages(targetDialogId)
					if (dialogIdRef.current === targetDialogId) {
						setLoading(false)
					}
				}
			} catch (err) {
				const aborted =
					controller.signal.aborted ||
					(err instanceof DOMException && err.name === 'AbortError')
				if (aborted) {
					setLoading(false)
					return
				}
				if (targetDialogId && dialogIdRef.current === targetDialogId) {
					if (
						await recoverDroppedTurn(
							err,
							targetDialogId,
							controller.signal,
							(msgs) => hasLastUserMessage(msgs, text),
						)
					) {
						setLoading(false)
						return
					}
					if (controller.signal.aborted) {
						setLoading(false)
						return
					}
					setError(extractErrorMessage(err))
					setMessages((prev) => {
						const last = prev[prev.length - 1]
						if (
							last?.role === 'user' &&
							last.content === text &&
							(last.attachments?.length ?? 0) === sentAttachments.length
						) {
							return prev.slice(0, -1)
						}
						return prev
					})
					if (sentAttachments.length > 0) {
						setPendingAttachments((prev) => [...prev, ...sentAttachments])
					}
				}
				setLoading(false)
			} finally {
				abortRef.current = null
			}
		},
		[
			loading,
			messagesLoading,
			hasPendingQuestions,
			ensureDialog,
			bumpDialogsVersion,
			applyChatResponse,
			reloadMessages,
			recoverDroppedTurn,
		],
	)

	const handleSend = useCallback(async () => {
		const text = draft.trim()
		const hasAttachments = pendingAttachments.length > 0
		if ((!text && !hasAttachments) || loading || messagesLoading || hasPendingQuestions) {
			return
		}

		setDraft('')
		const sentAttachments = [...pendingAttachments]
		setPendingAttachments([])
		await sendMessage(text, sentAttachments)
	}, [
		draft,
		loading,
		messagesLoading,
		hasPendingQuestions,
		pendingAttachments,
		sendMessage,
	])

	useEffect(() => {
		if (!activeDialogId) return
		if (loadedDialogIdRef.current !== activeDialogId) return
		// Every condition sendMessage bails out on has to be repeated here:
		// consuming a message it would then drop loses it for good.
		if (messagesLoading || loading || hasPendingQuestions) return

		const pending = pendingMessages.find((m) => m.dialogId === activeDialogId)
		if (!pending) return

		const consumed = consumePendingMessage(activeDialogId)
		if (!consumed) return

		void sendMessage(consumed.text)
	}, [
		activeDialogId,
		pendingMessages,
		messagesLoading,
		loading,
		hasPendingQuestions,
		sendMessage,
		consumePendingMessage,
	])

	const handleAskQuestionSubmit = useCallback(
		async (answers: string[]) => {
			const targetDialogId = dialogIdRef.current ?? activeDialogId
			const toolCallId = pendingToolCallId
			if (!targetDialogId || !toolCallId) {
				setError('Missing dialog or tool call for question answers')
				return
			}

			// Same shape `toUiMessages` rebuilds from the stored tool row, so the
			// reload that follows is idempotent rather than destructive.
			const answered = pendingQuestions.map((q, i) => ({
				question: q.question,
				answer: answers[i] ?? '',
			}))
			setMessages((prev) => [
				...prev,
				{ role: 'user' as const, content: '', answers: answered },
			])
			setPendingQuestions([])
			setPendingToolCallId(null)
			setSubmittingAnswers(true)
			setLoading(true)
			setError(null)

			const controller = new AbortController()
			abortRef.current = controller

			try {
				const resp = await submitToolResults(
					targetDialogId,
					toolCallId,
					answers,
					controller.signal,
				)
				bumpDialogsVersion()
				if (dialogIdRef.current !== targetDialogId) {
					setLoading(false)
					return
				}
				const awaitingInput = applyChatResponse(resp)
				if (!awaitingInput) {
					await reloadMessages(targetDialogId)
					if (dialogIdRef.current === targetDialogId) {
						setLoading(false)
					}
				}
			} catch (err) {
				const aborted =
					controller.signal.aborted ||
					(err instanceof DOMException && err.name === 'AbortError')
				if (aborted) {
					setLoading(false)
					return
				}
				if (dialogIdRef.current === targetDialogId) {
					const recovered = await recoverDroppedTurn(
						err,
						targetDialogId,
						controller.signal,
						(msgs) => hasToolResult(msgs, toolCallId),
					)
					if (!recovered && !controller.signal.aborted) {
						setError(extractErrorMessage(err))
					}
					setLoading(false)
				}
			} finally {
				abortRef.current = null
				setSubmittingAnswers(false)
			}
		},
		[
			activeDialogId,
			pendingToolCallId,
			pendingQuestions,
			applyChatResponse,
			bumpDialogsVersion,
			reloadMessages,
			recoverDroppedTurn,
		],
	)

	const handleStop = useCallback(() => {
		abortRef.current?.abort()
	}, [])

	const handleRetry = useCallback(async () => {
		const targetDialogId = dialogIdRef.current ?? activeDialogId
		if (!targetDialogId || loading || messagesLoading || hasPendingQuestions) {
			return
		}

		const lastUserIndex = findLastIndex(messages, isUserTurn)
		if (lastUserIndex < 0) {
			return
		}
		const hasAssistantAfterUser = messages.some(
			(m, i) => i > lastUserIndex && m.role === 'assistant',
		)
		if (!hasAssistantAfterUser) {
			return
		}

		setError(null)
		setMessages((prev) => {
			const idx = findLastIndex(prev, isUserTurn)
			if (idx < 0) {
				return prev
			}
			return prev.slice(0, idx + 1)
		})
		setLoading(true)

		const controller = new AbortController()
		abortRef.current = controller

		try {
			const resp = await retryLastResponse(targetDialogId, controller.signal)
			bumpDialogsVersion()
			if (dialogIdRef.current !== targetDialogId) {
				setLoading(false)
				return
			}
			const awaitingInput = applyChatResponse(resp)
			if (!awaitingInput) {
				await reloadMessages(targetDialogId)
				if (dialogIdRef.current === targetDialogId) {
					setLoading(false)
				}
			}
		} catch (err) {
			const aborted =
				controller.signal.aborted ||
				(err instanceof DOMException && err.name === 'AbortError')
			if (aborted) {
				setLoading(false)
				return
			}
			if (dialogIdRef.current === targetDialogId) {
				if (
					await recoverDroppedTurn(err, targetDialogId, controller.signal)
				) {
					setLoading(false)
					return
				}
				if (!controller.signal.aborted) {
					setError(extractErrorMessage(err))
				}
				try {
					const msgs = await getDialogMessages(targetDialogId)
					if (dialogIdRef.current === targetDialogId) {
						applyServerMessages(msgs)
					}
				} catch (syncErr) {
					setError(extractErrorMessage(syncErr))
				}
				setLoading(false)
			}
		} finally {
			abortRef.current = null
		}
	}, [
		activeDialogId,
		loading,
		messagesLoading,
		hasPendingQuestions,
		messages,
		applyChatResponse,
		applyServerMessages,
		bumpDialogsVersion,
		reloadMessages,
		recoverDroppedTurn,
	])

	const handleAttachClick = useCallback(() => {
		if (inputDisabled || uploading) return
		fileInputRef.current?.click()
	}, [inputDisabled, uploading])

	const handleFilesSelected = useCallback(
		async (e: React.ChangeEvent<HTMLInputElement>) => {
			const files = e.target.files
			if (!files?.length || inputDisabled || uploading) return

			setUploading(true)
			setError(null)
			try {
				const dialogId = await ensureDialog()
				const uploaded: Attachment[] = []
				for (const file of Array.from(files)) {
					const attachment = await uploadAttachment(dialogId, file)
					uploaded.push(attachment)
				}
				setPendingAttachments((prev) => [...prev, ...uploaded])
			} catch (err) {
				setError(extractErrorMessage(err))
			} finally {
				setUploading(false)
				if (fileInputRef.current) {
					fileInputRef.current.value = ''
				}
			}
		},
		[inputDisabled, uploading, ensureDialog],
	)

	const handleRemoveAttachment = useCallback(
		async (attachment: Attachment) => {
			setPendingAttachments((prev) =>
				prev.filter((a) => a.id !== attachment.id),
			)
			const dialogId = dialogIdRef.current ?? activeDialogId
			if (!dialogId) return
			try {
				await deleteAttachment(dialogId, attachment.id)
			} catch (err) {
				setError(extractErrorMessage(err))
				setPendingAttachments((prev) => [...prev, attachment])
			}
		},
		[activeDialogId],
	)

	function handleDraftChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
		const prev = draft
		const next = e.target.value
		setDraft(next)
		setActiveSkillIndex(0)
		if (shouldRearmSkillMenu(prev, next)) {
			setSkillMenuDismissed(false)
		}
	}

	function handleTextareaFocus() {
		setSkillMenuDismissed(false)
	}

	function handleTextareaBlur(e: React.FocusEvent<HTMLTextAreaElement>) {
		const related = e.relatedTarget
		if (
			related instanceof HTMLElement &&
			related.closest(`#${SKILL_LISTBOX_ID}`)
		) {
			return
		}
		setSkillMenuDismissed(true)
	}

	const handleSelectSkill = useCallback((name: string) => {
		setDraft(applySkillSelection(name))
		setSkillMenuDismissed(false)
		setActiveSkillIndex(0)
		// The row's mousedown kept focus in the textarea, but the caret still has to
		// be pushed past the text React just wrote.
		requestAnimationFrame(() => {
			const el = textareaRef.current
			if (!el) return
			el.focus()
			el.setSelectionRange(el.value.length, el.value.length)
		})
	}, [])

	function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
		// An IME candidate window owns Enter and the arrows while composing.
		if (e.nativeEvent.isComposing) {
			return
		}

		if (isSkillMenuOpen) {
			if (e.key === 'ArrowDown') {
				e.preventDefault()
				setActiveSkillIndex((i) => (i + 1) % skillSuggestions.length)
				return
			}
			if (e.key === 'ArrowUp') {
				e.preventDefault()
				setActiveSkillIndex(
					(i) => (i - 1 + skillSuggestions.length) % skillSuggestions.length,
				)
				return
			}
			if (e.key === 'Escape') {
				e.preventDefault()
				setSkillMenuDismissed(true)
				return
			}
			// Enter accepts the suggestion rather than sending: the draft is still
			// only the command token, so there is nothing worth sending yet.
			if ((e.key === 'Enter' && !e.shiftKey) || e.key === 'Tab') {
				const picked = skillSuggestions[activeSkillIndex]
				if (picked) {
					e.preventDefault()
					handleSelectSkill(picked.name)
					return
				}
			}
		}

		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault()
			void handleSend()
		}
	}

	const canSend =
		(draft.trim().length > 0 || pendingAttachments.length > 0) &&
		!inputDisabled

	const lastUserIndex = findLastIndex(messages, isUserTurn)
	const lastAssistantIndex = findLastIndex(
		messages,
		(m) => m.role === 'assistant',
	)
	const canRetry =
		!loading &&
		!messagesLoading &&
		!hasPendingQuestions &&
		lastAssistantIndex >= 0 &&
		lastAssistantIndex > lastUserIndex

	function renderAttachmentChip(attachment: Attachment, onRemove?: () => void) {
		const isImage = attachment.kind === 'image'
		return (
			<div
				key={attachment.id}
				className="flex items-center gap-1.5 rounded-md border bg-muted/50 px-2 py-1 text-xs"
			>
				{isImage && attachment.url ? (
					<img
						src={attachment.url}
						alt={attachment.filename}
						className="h-8 w-8 rounded object-cover"
					/>
				) : (
					<FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
				)}
				<span className="max-w-[120px] truncate" title={attachment.filename}>
					{attachment.filename}
				</span>
				{onRemove && (
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								onClick={onRemove}
								className={cn(
									'flex size-5 cursor-pointer items-center justify-center',
									'rounded transition-colors duration-[var(--dur-fast)]',
									'focus-ring hover:bg-foreground/[0.1]',
								)}
								aria-label={`Remove ${attachment.filename}`}
							>
								<X className="h-3 w-3" />
							</button>
						</TooltipTrigger>
						<TooltipContent>Remove {attachment.filename}</TooltipContent>
					</Tooltip>
				)}
			</div>
		)
	}

	function renderMessageAttachments(attachments?: Attachment[]) {
		if (!attachments?.length) return null
		return (
			<div className="mt-1.5 flex flex-wrap gap-1.5">
				{attachments.map((a) => renderAttachmentChip(a))}
			</div>
		)
	}

	return (
		<div ref={panelRef} className="flex h-full min-h-0 min-w-0 flex-col">
			<ChatHistoryMenu />

			<div
				ref={scrollRef}
				className="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden p-4"
			>
				{messagesLoading && (
					<div className="flex items-center gap-2 text-sm text-muted-foreground">
						<Loader2 className="h-3.5 w-3.5 animate-spin" />
						Loading conversation…
					</div>
				)}

				{!messagesLoading &&
					messages.length === 0 &&
					!loading &&
					!hasPendingQuestions && (
						<p className="text-sm text-muted-foreground">
							{activeDialogId
								? 'No messages yet. Send a message to start.'
								: 'Messages will appear here. Select a dialog or send a message to start a new one.'}
						</p>
					)}

				<div className="space-y-4">
					{messages.map((msg, i) =>
						msg.table ? (
							<div key={i}>
								<TableMessage table={msg.table} />
								{canRetry && i === lastAssistantIndex && (
									<div className="mt-2 flex justify-end pr-2">
										<Button
											type="button"
											variant="ghost"
											size="sm"
											className="h-7 gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground"
											onClick={() => void handleRetry()}
										>
											<RotateCcw className="h-3 w-3" />
											Retry
										</Button>
									</div>
								)}
							</div>
						) : (
							<div
								key={i}
								className={cn(
									'flex min-w-0 gap-2',
									msg.role === 'user'
										? 'flex-row-reverse'
										: 'flex-row',
								)}
							>
								<div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border bg-muted">
									{msg.role === 'user' ? (
										<User className="h-3.5 w-3.5" />
									) : (
										<Bot className="h-3.5 w-3.5" />
									)}
								</div>
								<div
									className={cn(
										'min-w-0 max-w-[85%] overflow-hidden rounded-lg px-3 py-2 text-sm',
										msg.role === 'user' && 'whitespace-pre-wrap break-words',
										msg.role === 'user'
											? 'bg-primary text-primary-foreground'
											: 'bg-muted',
									)}
								>
									{msg.answers ? (
										<AnsweredQuestionsMessage answers={msg.answers} />
									) : msg.role === 'assistant' && msg.content ? (
										<MarkdownMessage
											content={msg.content}
											onDialogLink={handleOpenDialogLink}
										/>
									) : (
										msg.content
									)}
									{renderMessageAttachments(msg.attachments)}
									{msg.role === 'assistant' && msg.content && (
										<div className="mt-2 flex items-center justify-between gap-2 border-t border-border/50 pt-2">
											{canRetry && i === lastAssistantIndex ? (
												<Button
													type="button"
													variant="ghost"
													size="sm"
													className="h-7 gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground"
													onClick={() => void handleRetry()}
												>
													<RotateCcw className="h-3 w-3" />
													Retry
												</Button>
											) : (
												<span />
											)}
											<CopyButton text={msg.content} />
										</div>
									)}
								</div>
							</div>
						),
					)}

					{loading && !hasPendingQuestions && (
						<div className="flex min-w-0 items-center gap-2">
							<div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border bg-muted">
								<Bot className="h-3.5 w-3.5" />
							</div>
							<div className="flex min-w-0 max-w-[85%] items-center gap-2 rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">
								<Loader2 className="h-3.5 w-3.5 animate-spin" />
								{reconnecting
									? 'Connection dropped, waiting for the agent to finish…'
									: callingTool
										? (
											<span className="inline-flex items-center gap-1">
												Calling tool
												<AnimatedDots />
											</span>
										)
										: `${thinkingPhrase}...`}
							</div>
						</div>
					)}

					{hasPendingQuestions && (
						<AskQuestionMessage
							questions={pendingQuestions}
							onSubmit={(answers) =>
								void handleAskQuestionSubmit(answers)
							}
							submitting={submittingAnswers}
						/>
					)}
				</div>
			</div>

			{error && (
				<div
					role="alert"
					className="flex shrink-0 items-start gap-2 border-t border-destructive/35 bg-destructive/12 px-4 py-2 text-xs leading-relaxed text-destructive"
				>
					<AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
					<span className="min-w-0 whitespace-normal break-words">{error}</span>
				</div>
			)}

			<div className="relative shrink-0 border-t p-3">
				{isSkillMenuOpen && (
					<SkillSuggestMenu
						suggestions={skillSuggestions}
						activeIndex={activeSkillIndex}
						onActiveIndexChange={setActiveSkillIndex}
						onSelect={handleSelectSkill}
					/>
				)}
				{pendingAttachments.length > 0 && (
					<div className="mb-2 flex flex-wrap gap-1.5">
						{pendingAttachments.map((a) =>
							renderAttachmentChip(a, () => void handleRemoveAttachment(a)),
						)}
					</div>
				)}
				<div className="flex min-w-0 gap-2">
					<input
						ref={fileInputRef}
						type="file"
						className="hidden"
						multiple
						accept="image/*,text/*,.md,.txt,.csv,.json,.log,.yaml,.yml"
						onChange={(e) => void handleFilesSelected(e)}
					/>
					{/* The frame carries the vertical padding so the textarea's own box is
					    whole lines only — padding scrolls with the text and would show a
					    sliver of the next line under the border once the draft overflows. */}
					<div
						ref={composerFrameRef}
						className={cn(
							fieldClasses,
							'flex min-w-0 flex-1 flex-col py-2',
							'focus-ring-within focus-within:border-ring/60',
							inputDisabled && 'cursor-not-allowed bg-muted/30 opacity-60',
						)}
					>
						{/* Bare textarea: the frame above is the field surface and owns the
						    focus ring, so this must not bring one of its own. */}
						<textarea
							ref={textareaRef}
							value={draft}
							onChange={handleDraftChange}
							onKeyDown={handleKeyDown}
							onFocus={handleTextareaFocus}
							onBlur={handleTextareaBlur}
							placeholder="Message…"
							rows={2}
							disabled={inputDisabled}
							// No flex-1: a flex-basis of 0 would override the inline height
							// the autosize hook sets, pinning the composer to one row.
							className={cn(
								'min-h-[60px] w-full min-w-0 resize-none overflow-hidden',
								'border-0 bg-transparent px-3 py-0 text-sm text-foreground',
								'outline-none placeholder:text-muted-foreground/70',
								'disabled:cursor-not-allowed',
							)}
							role="combobox"
							aria-autocomplete="list"
							aria-expanded={isSkillMenuOpen}
							aria-controls={SKILL_LISTBOX_ID}
							aria-activedescendant={
								isSkillMenuOpen ? skillOptionId(activeSkillIndex) : undefined
							}
						/>
					</div>
					<div className="flex shrink-0 flex-col gap-2 self-end">
						<IconButton
							type="button"
							variant="outline"
							className="h-9 w-9"
							disabled={inputDisabled || uploading}
							onClick={handleAttachClick}
							tooltip="Attach file"
						>
							{uploading ? (
								<Loader2 className="h-4 w-4 animate-spin" />
							) : (
								<Paperclip className="h-4 w-4" />
							)}
						</IconButton>
						{loading ? (
							<IconButton
								type="button"
								variant="destructive"
								className="h-9 w-9"
								onClick={handleStop}
								tooltip="Stop generating"
							>
								<Square className="h-4 w-4" />
							</IconButton>
						) : (
							<IconButton
								type="button"
								className="h-9 w-9"
								disabled={!canSend}
								onClick={() => void handleSend()}
								tooltip="Send message"
							>
								<Send className="h-4 w-4" />
							</IconButton>
						)}
					</div>
				</div>
			</div>

		</div>
	)
}
