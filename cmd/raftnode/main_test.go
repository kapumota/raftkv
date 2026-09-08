package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kapumota/raftkv/internal/raft"
)

type stubProposalNode struct {
	err error
}

func (s stubProposalNode) Propose(ctx context.Context, cmd raft.Command) error {
	return s.err
}

type blockingProposalNode struct{}

func (blockingProposalNode) Propose(ctx context.Context, cmd raft.Command) error {
	<-ctx.Done()
	return ctx.Err()
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("no se pudo decodificar la respuesta JSON: %v", err)
	}
	return body
}

func TestKVSetReturnsConfirmedAfterCommit(t *testing.T) {
	handler := newKVSetHandler(stubProposalNode{}, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(`{"key":"saldo","value":"100"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusOK)
	}
	body := decodeResponse(t, recorder)
	if body["estado"] != "confirmado" {
		t.Fatalf("estado inesperado: se obtuvo %q, se esperaba %q", body["estado"], "confirmado")
	}
}

func TestKVSetReturnsServiceUnavailableWhenNotLeader(t *testing.T) {
	handler := newKVSetHandler(stubProposalNode{err: raft.ErrNotLeader}, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(`{"key":"saldo","value":"100"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusServiceUnavailable)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "no_es_lider" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "no_es_lider")
	}
}

func TestKVSetReturnsGatewayTimeoutWhenCommitTimesOut(t *testing.T) {
	handler := newKVSetHandler(blockingProposalNode{}, 10*time.Millisecond)
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(`{"key":"saldo","value":"100"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusGatewayTimeout)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "tiempo_de_espera_agotado" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "tiempo_de_espera_agotado")
	}
}

func TestKVSetReturnsRequestTimeoutWhenRequestIsCanceled(t *testing.T) {
	handler := newKVSetHandler(stubProposalNode{err: context.Canceled}, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(`{"key":"saldo","value":"100"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestTimeout {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusRequestTimeout)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "solicitud_cancelada" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "solicitud_cancelada")
	}
}

func TestKVSetRejectsInvalidJSON(t *testing.T) {
	handler := newKVSetHandler(stubProposalNode{}, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(`{"key":`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusBadRequest)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "solicitud_invalida" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "solicitud_invalida")
	}
}

func TestKVSetRejectsUnsupportedMethod(t *testing.T) {
	handler := newKVSetHandler(stubProposalNode{}, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/kv/set", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("cabecera Allow inesperada: se obtuvo %q, se esperaba %q", recorder.Header().Get("Allow"), http.MethodPost)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "metodo_no_permitido" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "metodo_no_permitido")
	}
}

func TestKVSetReturnsInternalErrorForUnexpectedFailure(t *testing.T) {
	handler := newKVSetHandler(stubProposalNode{err: errors.New("fallo inesperado")}, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/kv/set", strings.NewReader(`{"key":"saldo","value":"100"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusInternalServerError)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "error_interno" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "error_interno")
	}
}
