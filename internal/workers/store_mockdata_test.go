package workers

import (
	"context"
	"database/sql"
	"reflect"

	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

type MockStore struct {
	ctrl     *gomock.Controller
	recorder *MockStoreMockRecorder
}

type MockStoreMockRecorder struct {
	mock *MockStore
}

func NewMockStore(ctrl *gomock.Controller) *MockStore {
	mock := &MockStore{ctrl: ctrl}
	mock.recorder = &MockStoreMockRecorder{mock: mock}
	return mock
}

func (m *MockStore) EXPECT() *MockStoreMockRecorder {
	return m.recorder
}

func (m *MockStore) EnsureTask(ctx context.Context, arg sqlcgen.EnsureTaskParams) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnsureTask", ctx, arg)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockStoreMockRecorder) EnsureTask(ctx, arg any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnsureTask", reflect.TypeOf((*MockStore)(nil).EnsureTask), ctx, arg)
}

func (m *MockStore) ClaimTask(ctx context.Context, arg sqlcgen.ClaimTaskParams) (sql.Result, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ClaimTask", ctx, arg)
	ret0, _ := ret[0].(sql.Result)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockStoreMockRecorder) ClaimTask(ctx, arg any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ClaimTask", reflect.TypeOf((*MockStore)(nil).ClaimTask), ctx, arg)
}

func (m *MockStore) FinishTask(ctx context.Context, arg sqlcgen.FinishTaskParams) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FinishTask", ctx, arg)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockStoreMockRecorder) FinishTask(ctx, arg any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FinishTask", reflect.TypeOf((*MockStore)(nil).FinishTask), ctx, arg)
}

func (m *MockStore) DeleteEndedSessionsOlderThan(ctx context.Context, arg sqlcgen.DeleteEndedSessionsOlderThanParams) (sql.Result, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteEndedSessionsOlderThan", ctx, arg)
	ret0, _ := ret[0].(sql.Result)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (mr *MockStoreMockRecorder) DeleteEndedSessionsOlderThan(ctx, arg any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteEndedSessionsOlderThan", reflect.TypeOf((*MockStore)(nil).DeleteEndedSessionsOlderThan), ctx, arg)
}
