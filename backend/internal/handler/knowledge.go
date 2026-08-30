package handler

import (
	"net/http"
	"time"

	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/service"
)

const knowledgeMaxUploadBytes = 32 << 20

// KnowledgeHandler exposes HTTP endpoints for the config-backed knowledge base.
type KnowledgeHandler struct {
	svc *service.KnowledgeService
}

func NewKnowledgeHandler(svc *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

type connectionURIRequest struct {
	ConnectionURI string `json:"connectionUri"`
}

func (h *KnowledgeHandler) GetConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := h.svc.GetConnection(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func (h *KnowledgeHandler) UpdateConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req connectionURIRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.ConnectionURI == "" {
			writeError(w, http.StatusBadRequest, "connectionUri is required")
			return
		}

		conn, err := h.svc.SetConnection(r.Context(), req.ConnectionURI)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func (h *KnowledgeHandler) UploadDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(knowledgeMaxUploadBytes); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		content, err := service.ReadUploadContent(file, knowledgeMaxUploadBytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		collection := r.FormValue("collection")

		result, err := h.svc.UploadDocument(r.Context(), collection, header.Filename, content)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// knowledgeDocumentResponse is the current source document behind a collection.
// Filename and UpdatedAt are absent for the built-in template: nothing has been
// uploaded, so there is no file to name or date.
type knowledgeDocumentResponse struct {
	Collection string     `json:"collection"`
	Filename   string     `json:"filename,omitempty"`
	Content    string     `json:"content"`
	Source     string     `json:"source"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

func (h *KnowledgeHandler) GetDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collection := r.URL.Query().Get("collection")

		doc, err := h.svc.GetDocument(r.Context(), collection)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		resp := knowledgeDocumentResponse{
			Collection: doc.Collection,
			Content:    doc.Content,
			Source:     string(doc.Source),
		}
		if doc.Source == kbdoc.SourceUploaded {
			if !doc.UpdatedAt.IsZero() {
				updatedAt := doc.UpdatedAt.UTC()
				resp.UpdatedAt = &updatedAt
			}
			// The original filename lives in the vector store, not on disk.
			// Losing it must not cost the operator the document itself, so a
			// status failure is ignored rather than surfaced.
			if status, statusErr := h.svc.Status(r.Context(), doc.Collection); statusErr == nil {
				resp.Filename = status.SourceURI
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *KnowledgeHandler) GetStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collection := r.URL.Query().Get("collection")
		status, err := h.svc.Status(r.Context(), collection)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func (h *KnowledgeHandler) ListCollections() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collections, err := h.svc.ListCollections(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, collections)
	}
}
