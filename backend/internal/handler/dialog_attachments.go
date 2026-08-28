package handler

import (
	"errors"
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
)

const dialogAttachmentMaxBytes = 16 << 20

func (h *DialogHandler) UploadAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		if err := r.ParseMultipartForm(dialogAttachmentMaxBytes); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		content, err := service.ReadUploadContent(file, dialogAttachmentMaxBytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		attachment, err := h.chatSvc.SaveAttachment(r.Context(), id, header.Filename, header.Header.Get("Content-Type"), content)
		if err != nil {
			if errors.Is(err, service.ErrUnsupportedAttachmentType) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, attachment)
	}
}

func (h *DialogHandler) ListAttachments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		attachments, err := h.chatSvc.ListAttachments(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if attachments == nil {
			attachments = []domain.Attachment{}
		}
		writeJSON(w, http.StatusOK, attachments)
	}
}

func (h *DialogHandler) GetAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dialogID, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		attachmentID, ok := parseUUIDParam(w, r, "attachmentId", "attachment id")
		if !ok {
			return
		}

		attachment, content, err := h.chatSvc.ReadAttachmentContent(r.Context(), dialogID, attachmentID)
		if err != nil {
			if errors.Is(err, service.ErrAttachmentNotFound) {
				writeError(w, http.StatusNotFound, "attachment not found")
				return
			}
			handleServiceError(w, err)
			return
		}

		ct := attachment.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", `inline; filename="`+attachment.Filename+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}
}

func (h *DialogHandler) DeleteAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dialogID, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		attachmentID, ok := parseUUIDParam(w, r, "attachmentId", "attachment id")
		if !ok {
			return
		}

		if err := h.chatSvc.DeleteAttachment(r.Context(), dialogID, attachmentID); err != nil {
			if errors.Is(err, service.ErrAttachmentNotFound) {
				writeError(w, http.StatusNotFound, "attachment not found")
				return
			}
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
