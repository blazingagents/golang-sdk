package blazingagents

import (
	"context"
	"iter"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Task is a prompt an agent runs on a schedule or on demand.
type Task struct {
	ID       TaskID   `json:"id"`
	Schedule Schedule `json:"schedule"` // nil when the task runs only on demand
}

// TaskListItem is a task in a list.
type TaskListItem struct {
	ID       TaskID   `json:"id"`
	Schedule Schedule `json:"schedule"`
}

// CreatedTask is a new task and the run it started when submitted.
type CreatedTask struct {
	Task  Task       `json:"task"`
	RunID *TaskRunID `json:"runId"`
}

// CreatedTaskRun is a submitted run.
type CreatedTaskRun struct {
	RunID TaskRunID `json:"runId"`
}

// TaskRun is one run of a task.
type TaskRun struct {
	ID          TaskRunID   `json:"id"`
	AgentConfig AgentConfig `json:"agentConfig"`
}

// TaskRunMessageList is one page of a run's messages.
type TaskRunMessageList struct {
	Data       []Message `json:"data"`
	NextCursor string    `json:"nextCursor"`
}

func (l *TaskRunMessageList) items() []Message { return l.Data }
func (l *TaskRunMessageList) next() string     { return l.NextCursor }

// ListTasksParams filters listTasks.
type ListTasksParams struct {
	AgentID *AgentID `json:"-" query:"agentId"`
	UserID  *string  `json:"-" query:"userId"`
	Cursor  *string  `json:"-" query:"cursor"`
	Limit   *int     `json:"-" query:"limit"`
}

// CreateTaskParams is the body of createTask.
type CreateTaskParams struct {
	IdempotencyKey *string            `json:"idempotencyKey,omitzero"`
	AgentID        AgentID            `json:"agentId"`
	Name           string             `json:"name"`
	Prompt         string             `json:"prompt"`
	Schedule       Nullable[Schedule] `json:"schedule,omitzero"`
	Enabled        *bool              `json:"enabled,omitzero"`
	Submit         *bool              `json:"submit,omitzero"`
	UserID         *string            `json:"userId,omitzero"`
	Metadata       Metadata           `json:"metadata,omitzero"`
}

// UpdateTaskParams is the body of updateTask.
type UpdateTaskParams struct {
	Name     *string            `json:"name,omitzero"`
	Prompt   *string            `json:"prompt,omitzero"`
	Schedule Nullable[Schedule] `json:"schedule,omitzero"`
	Enabled  *bool              `json:"enabled,omitzero"`
	Metadata Metadata           `json:"metadata,omitzero"`
}

// SubmitTaskParams is the body of createTaskRun.
type SubmitTaskParams struct {
	IdempotencyKey *string `json:"idempotencyKey,omitzero"`
}

// ListTaskRunsParams pages listTaskRuns.
type ListTaskRunsParams struct {
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// ListTaskRunMessagesParams pages listTaskRunMessages; set Cursor or After, not both.
type ListTaskRunMessagesParams struct {
	After  *string `json:"-" query:"after"`
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// List is listTasks.
func (s *TasksService) List(ctx context.Context, params ListTasksParams, opts ...RequestOption) (*Page[TaskListItem], error) {
	return decode[Page[TaskListItem]](s.s.send(ctx, op.ListTasks(), params, opts))
}

// All iterates listTasks across pages.
func (s *TasksService) All(ctx context.Context, params ListTasksParams, opts ...RequestOption) iter.Seq2[TaskListItem, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[TaskListItem], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create is createTask. A 201 and a 202 both decode.
func (s *TasksService) Create(ctx context.Context, params CreateTaskParams, opts ...RequestOption) (*CreatedTask, error) {
	return decode[CreatedTask](s.s.send(ctx, op.CreateTask(), params, opts))
}

// Get is getTask.
func (s *TasksService) Get(ctx context.Context, taskID TaskID, opts ...RequestOption) (*Task, error) {
	return decode[Task](s.s.send(ctx, op.GetTask(string(taskID)), nil, opts))
}

// Update is updateTask.
func (s *TasksService) Update(ctx context.Context, taskID TaskID, params UpdateTaskParams, opts ...RequestOption) (*Task, error) {
	return decode[Task](s.s.send(ctx, op.UpdateTask(string(taskID)), params, opts))
}

// Delete is deleteTask.
func (s *TasksService) Delete(ctx context.Context, taskID TaskID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteTask(string(taskID)), nil, opts))
}

// Submit is createTaskRun.
func (s *TasksService) Submit(ctx context.Context, taskID TaskID, params SubmitTaskParams, opts ...RequestOption) (*CreatedTaskRun, error) {
	return decode[CreatedTaskRun](s.s.send(ctx, op.CreateTaskRun(string(taskID)), params, opts))
}

// ListRuns is listTaskRuns.
func (s *TasksService) ListRuns(ctx context.Context, taskID TaskID, params ListTaskRunsParams, opts ...RequestOption) (*Page[TaskRun], error) {
	return decode[Page[TaskRun]](s.s.send(ctx, op.ListTaskRuns(string(taskID)), params, opts))
}

// AllRuns iterates listTaskRuns across pages.
func (s *TasksService) AllRuns(ctx context.Context, taskID TaskID, params ListTaskRunsParams, opts ...RequestOption) iter.Seq2[TaskRun, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[TaskRun], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListRuns(ctx, taskID, p, opts...)
	})
}

// GetRun is getTaskRun.
func (s *TasksService) GetRun(ctx context.Context, taskID TaskID, runID TaskRunID, opts ...RequestOption) (*TaskRun, error) {
	return decode[TaskRun](s.s.send(ctx, op.GetTaskRun(string(taskID), string(runID)), nil, opts))
}

// ListRunMessages is listTaskRunMessages.
func (s *TasksService) ListRunMessages(ctx context.Context, taskID TaskID, runID TaskRunID, params ListTaskRunMessagesParams, opts ...RequestOption) (*TaskRunMessageList, error) {
	return decode[TaskRunMessageList](s.s.send(ctx, op.ListTaskRunMessages(string(taskID), string(runID)), params, opts))
}

// AllRunMessages iterates listTaskRunMessages across pages, keeping the direction: with
// After set, each next cursor goes into After, else into Cursor.
func (s *TasksService) AllRunMessages(ctx context.Context, taskID TaskID, runID TaskRunID, params ListTaskRunMessagesParams, opts ...RequestOption) iter.Seq2[Message, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*TaskRunMessageList, error) {
		p := params
		switch {
		case cursor == "":
		case params.After != nil:
			p.After = &cursor
		default:
			p.Cursor = &cursor
		}
		return s.ListRunMessages(ctx, taskID, runID, p, opts...)
	})
}

// CancelRun is cancelTaskRun.
func (s *TasksService) CancelRun(ctx context.Context, taskID TaskID, runID TaskRunID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.CancelTaskRun(string(taskID), string(runID)), nil, opts))
}
