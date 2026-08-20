package app

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"pokernode/internal/store"
)

const maxSpaceMessageRunes = 500

type spaceMessageInput struct {
	Body string `json:"body"`
}

func (s *Server) handleListSpaceMessages(w http.ResponseWriter, r *http.Request, user store.User) error {
	spaceID := r.PathValue("spaceID")
	if _, err := s.store.SpaceForUser(r.Context(), spaceID, user.ID); err != nil {
		return err
	}
	afterID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return &apiError{Status: http.StatusBadRequest, Message: "消息游标无效"}
		}
		afterID = parsed
	}
	messages, err := s.store.SpaceMessages(r.Context(), spaceID, afterID)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
	return nil
}

func (s *Server) handleCreateSpaceMessage(w http.ResponseWriter, r *http.Request, user store.User) error {
	spaceID := r.PathValue("spaceID")
	if _, err := s.store.SpaceForUser(r.Context(), spaceID, user.ID); err != nil {
		return err
	}
	var input spaceMessageInput
	if err := decodeJSON(r, &input); err != nil {
		return err
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" {
		return &apiError{Status: http.StatusBadRequest, Message: "消息不能为空"}
	}
	if utf8.RuneCountInString(input.Body) > maxSpaceMessageRunes {
		return &apiError{Status: http.StatusBadRequest, Message: "消息不能超过 500 个字符"}
	}
	message, err := s.store.CreateSpaceMessage(r.Context(), spaceID, user.ID, input.Body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": message})
	return nil
}
