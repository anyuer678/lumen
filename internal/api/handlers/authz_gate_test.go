package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent/internal/auth"
)

func ctxWithLevel(level int) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	p := &auth.TokenPrincipal{PermLevel: level}
	return req.WithContext(auth.WithPrincipal(context.Background(), p))
}

func TestTokenListRequiresL3(t *testing.T) {
	h := &TokenHandler{}
	for _, level := range []int{0, 1, 2} {
		w := httptest.NewRecorder()
		h.List(w, ctxWithLevel(level))
		if w.Code != http.StatusForbidden {
			t.Errorf("List with L%d = %d, want 403", level, w.Code)
		}
	}
}

func TestTokenRevokeRequiresL3(t *testing.T) {
	h := &TokenHandler{}
	for _, level := range []int{0, 1, 2} {
		w := httptest.NewRecorder()
		req := ctxWithLevel(level)
		// chi.URLParam needs route context; empty id is fine — gate fires before DB
		h.Revoke(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("Revoke with L%d = %d, want 403", level, w.Code)
		}
	}
}

func TestEmitEventRequiresL2(t *testing.T) {
	h := &EventHandler{}
	for _, level := range []int{0, 1} {
		w := httptest.NewRecorder()
		h.EmitEvent(w, ctxWithLevel(level))
		if w.Code != http.StatusForbidden {
			t.Errorf("EmitEvent with L%d = %d, want 403", level, w.Code)
		}
	}
}

func TestClearEventsRequiresL3(t *testing.T) {
	h := &EventHandler{}
	for _, level := range []int{0, 1, 2} {
		w := httptest.NewRecorder()
		h.ClearOld(w, ctxWithLevel(level))
		if w.Code != http.StatusForbidden {
			t.Errorf("ClearOld with L%d = %d, want 403", level, w.Code)
		}
	}
}
