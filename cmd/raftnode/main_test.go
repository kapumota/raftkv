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

type stubReadNode struct {
	index int
	err   error
}

func (s stubReadNode) ReadIndex(ctx context.Context) (int, error) {
	return s.index, s.err
}

type blockingReadNode struct{}

func (blockingReadNode) ReadIndex(ctx context.Context) (int, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

type stubKeyValueReader struct {
	values map[string]string
	reads  int
}

func (s *stubKeyValueReader) Get(key string) (string, bool) {
	s.reads++
	value, ok := s.values[key]
	return value, ok
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

func TestKVGetReturnsValueAfterConfirmingLeadership(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{"saldo": "100"}}
	handler := newKVGetHandler(stubReadNode{index: 2}, store, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/kv/get?key=saldo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusOK)
	}
	body := decodeResponse(t, recorder)
	if body["key"] != "saldo" || body["value"] != "100" {
		t.Fatalf("respuesta inesperada: se obtuvo %+v", body)
	}
	if store.reads != 1 {
		t.Fatalf("cantidad inesperada de consultas al KV: se obtuvo %d, se esperaba 1", store.reads)
	}
}

func TestKVGetReturnsNotFoundAfterConfirmingLeadership(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{}}
	handler := newKVGetHandler(stubReadNode{index: 1}, store, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/kv/get?key=ausente", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusNotFound)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "no_encontrado" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "no_encontrado")
	}
	if store.reads != 1 {
		t.Fatalf("cantidad inesperada de consultas al KV: se obtuvo %d, se esperaba 1", store.reads)
	}
}

func TestKVGetReturnsServiceUnavailableWhenNotLeader(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{"saldo": "100"}}
	handler := newKVGetHandler(stubReadNode{err: raft.ErrNotLeader}, store, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/kv/get?key=saldo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusServiceUnavailable)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "no_es_lider" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "no_es_lider")
	}
	if store.reads != 0 {
		t.Fatalf("el KV fue consultado sin liderazgo: se obtuvo %d consultas", store.reads)
	}
}

func TestKVGetReturnsGatewayTimeoutWithoutQuorum(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{"saldo": "100"}}
	handler := newKVGetHandler(blockingReadNode{}, store, 10*time.Millisecond)
	req := httptest.NewRequest(http.MethodGet, "/kv/get?key=saldo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusGatewayTimeout)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "tiempo_de_espera_agotado" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "tiempo_de_espera_agotado")
	}
	if store.reads != 0 {
		t.Fatalf("el KV fue consultado sin quorum: se obtuvo %d consultas", store.reads)
	}
}

func TestKVGetReturnsRequestTimeoutWhenRequestIsCanceled(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{"saldo": "100"}}
	handler := newKVGetHandler(stubReadNode{err: context.Canceled}, store, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/kv/get?key=saldo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestTimeout {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusRequestTimeout)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "solicitud_cancelada" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "solicitud_cancelada")
	}
	if store.reads != 0 {
		t.Fatalf("el KV fue consultado con una solicitud cancelada: se obtuvo %d consultas", store.reads)
	}
}

func TestKVGetRejectsUnsupportedMethod(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{}}
	handler := newKVGetHandler(stubReadNode{}, store, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/kv/get?key=saldo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("cabecera Allow inesperada: se obtuvo %q, se esperaba %q", recorder.Header().Get("Allow"), http.MethodGet)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "metodo_no_permitido" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "metodo_no_permitido")
	}
	if store.reads != 0 {
		t.Fatalf("el KV fue consultado con un método no permitido: se obtuvo %d consultas", store.reads)
	}
}

func TestKVGetReturnsInternalErrorForUnexpectedFailure(t *testing.T) {
	store := &stubKeyValueReader{values: map[string]string{"saldo": "100"}}
	handler := newKVGetHandler(stubReadNode{err: errors.New("fallo inesperado")}, store, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/kv/get?key=saldo", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("código HTTP inesperado: se obtuvo %d, se esperaba %d", recorder.Code, http.StatusInternalServerError)
	}
	body := decodeResponse(t, recorder)
	if body["error"] != "error_interno" {
		t.Fatalf("error inesperado: se obtuvo %q, se esperaba %q", body["error"], "error_interno")
	}
	if store.reads != 0 {
		t.Fatalf("el KV fue consultado después de un fallo interno: se obtuvo %d consultas", store.reads)
	}
}
