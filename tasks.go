package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// Task is a prompt an agent runs on a schedule or on demand.
type Task struct {
	ID          TaskID     `json:"id"`
	TenantID    TenantID   `json:"tenantId"`
	AgentID     AgentID    `json:"agentId"`
	Name        string     `json:"name"`
	Prompt      string     `json:"prompt"`
	Schedule    Schedule   `json:"schedule"` // nil when the task runs only on demand
	Enabled     bool       `json:"enabled"`
	NextFireAt  *time.Time `json:"nextFireAt"`
	ActiveRunID *TaskRunID `json:"activeRunId"`
	LatestRunID *TaskRunID `json:"latestRunId"`
	UserID      string     `json:"userId"`
	Metadata    Metadata   `json:"metadata"`
	DeletedAt   *time.Time `json:"deletedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// TaskListItem is a task in a list, with a summary of its latest run.
type TaskListItem struct {
	ID          TaskID         `json:"id"`
	TenantID    TenantID       `json:"tenantId"`
	AgentID     AgentID        `json:"agentId"`
	Name        string         `json:"name"`
	Prompt      string         `json:"prompt"`
	Schedule    Schedule       `json:"schedule"` // nil when the task runs only on demand
	Enabled     bool           `json:"enabled"`
	NextFireAt  *time.Time     `json:"nextFireAt"`
	ActiveRunID *TaskRunID     `json:"activeRunId"`
	LatestRunID *TaskRunID     `json:"latestRunId"`
	UserID      string         `json:"userId"`
	Metadata    Metadata       `json:"metadata"`
	DeletedAt   *time.Time     `json:"deletedAt"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	LatestRun   *TaskLatestRun `json:"latestRun"` // nil when the task never ran
}

// TaskLatestRun is the summary of a task's latest run in a list.
type TaskLatestRun struct {
	ID         TaskRunID     `json:"id"`
	Status     TaskRunStatus `json:"status"`
	FinishedAt *time.Time    `json:"finishedAt"`
}

// CreatedTask is a new task and the run it started when submitted.
type CreatedTask struct {
	Task  Task       `json:"task"`
	RunID *TaskRunID `json:"runId"` // nil unless the task was created with Submit
}

// CreatedTaskRun is a submitted run.
type CreatedTaskRun struct {
	RunID TaskRunID `json:"runId"`
}

// TaskRun is one run of a task.
type TaskRun struct {
	ID                TaskRunID     `json:"id"`
	TaskID            TaskID        `json:"taskId"`
	TenantID          TenantID      `json:"tenantId"`
	AgentID           AgentID       `json:"agentId"`
	AgentConfig       AgentConfig   `json:"agentConfig"`
	SessionID         *SessionID    `json:"sessionId"`
	TurnID            *TurnID       `json:"turnId"`
	Status            TaskRunStatus `json:"status"`
	Error             *string       `json:"error"`
	ErrorCode         *ErrorCode    `json:"errorCode"` // why a blocked or failed run ended, such as ErrorCodeUsageCreditRequired; nil otherwise
	UserID            string        `json:"userId"`
	Metadata          Metadata      `json:"metadata"`
	StartedAt         *time.Time    `json:"startedAt"`
	FinishedAt        *time.Time    `json:"finishedAt"`
	CancelRequestedAt *time.Time    `json:"cancelRequestedAt"`
	CanceledAt        *time.Time    `json:"canceledAt"`
	CreatedAt         time.Time     `json:"createdAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
}

// TaskRunMessageList is one page of a run's messages with the run's state at that moment.
type TaskRunMessageList struct {
	Data         []Message     `json:"data"`
	NextCursor   string        `json:"nextCursor"`   // "" on the last page
	LatestCursor *string       `json:"latestCursor"` // pass it as After to poll for newer messages
	Status       TaskRunStatus `json:"status"`
	Error        *string       `json:"error"`
	FinishedAt   *time.Time    `json:"finishedAt"`
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

// UpdateTaskParams is the body of updateTask. An omitted field is left unchanged.
type UpdateTaskParams struct {
	Name     *string            `json:"name,omitzero"`
	Prompt   *string            `json:"prompt,omitzero"`
	Schedule Nullable[Schedule] `json:"schedule,omitzero"` // Null makes the task on-demand only
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

// ListTaskRunMessagesParams pages listTaskRunMessages. Cursor pages back through older
// messages and After pages forward from a LatestCursor; setting both is an *ArgumentError.
type ListTaskRunMessagesParams struct {
	After  *string `json:"-" query:"after"`
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// List returns one page of tasks (listTasks).
// See https://docs.blazingagents.com/api-reference/rest-api/tasks#list-tasks.
func (s *TasksService) List(ctx context.Context, params ListTasksParams, opts ...RequestOption) (*Page[TaskListItem], error) {
	return decode[Page[TaskListItem]](s.s.send(ctx, op.ListTasks(), params, opts))
}

// All iterates over every task from params.Cursor on, fetching pages as needed (listTasks).
// See https://docs.blazingagents.com/api-reference/rest-api/tasks#list-tasks.
func (s *TasksService) All(ctx context.Context, params ListTasksParams, opts ...RequestOption) iter.Seq2[TaskListItem, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[TaskListItem], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Create creates a task (createTask).
// With Submit set the server answers 202 and CreatedTask.RunID names the first run; both
// 201 and 202 decode.
// See https://docs.blazingagents.com/api-reference/rest-api/tasks#create-task.
func (s *TasksService) Create(ctx context.Context, params CreateTaskParams, opts ...RequestOption) (*CreatedTask, error) {
	return decode[CreatedTask](s.s.send(ctx, op.CreateTask(), params, opts))
}

// Get returns a task (getTask).
// See https://docs.blazingagents.com/api-reference/rest-api/tasks#get-task.
func (s *TasksService) Get(ctx context.Context, taskID TaskID, opts ...RequestOption) (*Task, error) {
	return decode[Task](s.s.send(ctx, op.GetTask(string(taskID)), nil, opts))
}

// Update changes a task (updateTask).
// See https://docs.blazingagents.com/api-reference/rest-api/tasks#update-task.
func (s *TasksService) Update(ctx context.Context, taskID TaskID, params UpdateTaskParams, opts ...RequestOption) (*Task, error) {
	return decode[Task](s.s.send(ctx, op.UpdateTask(string(taskID)), params, opts))
}

// Delete deletes a task (deleteTask).
// See https://docs.blazingagents.com/api-reference/rest-api/tasks#delete-task.
func (s *TasksService) Delete(ctx context.Context, taskID TaskID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteTask(string(taskID)), nil, opts))
}

// Submit queues a run of a task (createTaskRun).
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#create-task-run.
func (s *TasksService) Submit(ctx context.Context, taskID TaskID, params SubmitTaskParams, opts ...RequestOption) (*CreatedTaskRun, error) {
	return decode[CreatedTaskRun](s.s.send(ctx, op.CreateTaskRun(string(taskID)), params, opts))
}

// ListRuns returns one page of a task's runs (listTaskRuns).
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#list-task-runs.
func (s *TasksService) ListRuns(ctx context.Context, taskID TaskID, params ListTaskRunsParams, opts ...RequestOption) (*Page[TaskRun], error) {
	return decode[Page[TaskRun]](s.s.send(ctx, op.ListTaskRuns(string(taskID)), params, opts))
}

// AllRuns iterates over every run of a task from params.Cursor on, fetching pages as needed
// (listTaskRuns).
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#list-task-runs.
func (s *TasksService) AllRuns(ctx context.Context, taskID TaskID, params ListTaskRunsParams, opts ...RequestOption) iter.Seq2[TaskRun, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[TaskRun], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListRuns(ctx, taskID, p, opts...)
	})
}

// GetRun returns a task run (getTaskRun).
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#get-task-run.
func (s *TasksService) GetRun(ctx context.Context, taskID TaskID, runID TaskRunID, opts ...RequestOption) (*TaskRun, error) {
	return decode[TaskRun](s.s.send(ctx, op.GetTaskRun(string(taskID), string(runID)), nil, opts))
}

// ListRunMessages returns one page of a run's messages (listTaskRunMessages). Setting both
// params.Cursor and params.After is an *ArgumentError.
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#list-task-run-messages.
func (s *TasksService) ListRunMessages(ctx context.Context, taskID TaskID, runID TaskRunID, params ListTaskRunMessagesParams, opts ...RequestOption) (*TaskRunMessageList, error) {
	if params.Cursor != nil && params.After != nil {
		return nil, &ArgumentError{Name: "params.Cursor", Reason: "set together with params.After; use one"}
	}
	return decode[TaskRunMessageList](s.s.send(ctx, op.ListTaskRunMessages(string(taskID), string(runID)), params, opts))
}

// AllRunMessages iterates over a run's messages across pages, keeping the direction: with
// After set, each next cursor goes into After, else into Cursor (listTaskRunMessages).
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#list-task-run-messages.
func (s *TasksService) AllRunMessages(ctx context.Context, taskID TaskID, runID TaskRunID, params ListTaskRunMessagesParams, opts ...RequestOption) iter.Seq2[Message, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*TaskRunMessageList, error) {
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

// CancelRun asks a run to stop (cancelTaskRun). It returns once cancellation is requested;
// the run reaches canceled later.
// See https://docs.blazingagents.com/api-reference/rest-api/task-runs#cancel-task-run.
func (s *TasksService) CancelRun(ctx context.Context, taskID TaskID, runID TaskRunID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.CancelTaskRun(string(taskID), string(runID)), nil, opts))
}
