# Knowledge Base

The knowledge base is the project documentation the agent searches with the
`knowledge_search` tool. A document is uploaded on the **Knowledge Base** page,
split into chunks, embedded, and stored in pgvector under a named *collection*.

## Collections and projects

The collection name is the storage key. The Knowledge Base page defaults it to
the project selected in the header, and to `default` when no project is
selected — so each project keeps its own knowledge base. A project whose name is
not a usable collection name (`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`) falls back to
`default`. Any other collection can still be picked or created by hand in the
collection selector.

Uploading **replaces** every chunk in the collection: one document is kept at a
time.

## Where the file is stored

The uploaded file itself is written to `{DATA_DIR}/knowledgebase/{collection}.md`
(configurable with `knowledge_base.dir`) once the chunks are committed, and is
overwritten on re-upload. It lives on the same volume as the rest of `data/`, so
it survives container and image rebuilds.

This is what makes the document readable again: the vector store holds chunks,
not the original text.

## The built-in template

A skeleton is compiled into the binary
([`internal/kbdoc/defaults/skeleton.md`](../../backend/internal/kbdoc/defaults/skeleton.md)).
It is **never written to disk and never indexed** — it is only what the read
endpoint returns for a collection that has nothing uploaded yet, so an install
that has not uploaded anything keeps tracking the template of the current image.

Press **View current** on the Knowledge Base page to read it, then Download it,
fill it in, and upload it to index it. The template shows the raw source rather
than rendered markdown: that is exactly what gets chunked, and it keeps the
guidance that the template carries in HTML comments visible.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/knowledge/documents?collection=` | Current source document. Answers with the template when nothing was uploaded, so it never 404s. `source` is `uploaded` or `template`. |
| `POST` | `/api/v1/knowledge/documents` | Multipart upload (`file`, `collection`). Indexes and stores the file. |
| `GET` | `/api/v1/knowledge/collections` | All collections with chunk counts. |
| `GET` | `/api/v1/knowledge/status?collection=` | Chunk count and embedding metadata. |
| `GET`/`PUT` | `/api/v1/knowledge/connection` | The pgvector connection URI (`knowledge_base.uri`). |

## Writing a good knowledge base

Retrieval returns ~500-character chunks by semantic similarity, so each section
has to stand on its own without the ones around it. State facts rather than
prose, spell names out in full at least once (clusters, namespaces, hostnames,
repository URLs) because the agent matches on them literally, and never record
secret values — only where a secret lives.

The template's own comments carry this guidance, which is why **View current**
shows the raw source rather than rendered markdown.
