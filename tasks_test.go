package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func tasksIDs() (ba.TaskID, ba.TaskRunID, ba.AgentID) {
	return ba.TaskID(testkit.ID(op.IDPatterns["TaskID"])), ba.TaskRunID(testkit.ID(op.IDPatterns["TaskRunID"])), ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
}

// tasksNoUser is the tenant client's request: no user header on the wire.
var tasksNoUser = []string{"X-Ba-User-Id"}

// tasksCovers checks the decoded value against the spec example in both directions.
func tasksCovers(example string) func(*testing.T, any) {
	return func(t *testing.T, got any) {
		testkit.CoversExample(t, example, got)
		testkit.MatchesExample(t, example, got)
	}
}

// tasksAPIError checks the error of a failed call: the type, its fields and its message.
func tasksAPIError(t *testing.T, err error, opID string, status int, code ba.ErrorCode, message string) {
	t.Helper()
	var e *ba.APIError
	if !errors.As(err, &e) {
		t.Fatalf("error %T %v, want *APIError", err, err)
	}
	if e.Status != status || e.Code != code || e.Message != message || e.Meta.OperationID != opID || e.Meta.Status != status {
		t.Fatalf("decoded %+v", e)
	}
	if want := "blazingagents: " + opID + ": " + strconv.Itoa(status) + " " + string(code) + ": " + message; err.Error() != want {
		t.Fatalf("message %q, want %q", err, want)
	}
}

func TestTasksRequests(t *testing.T) {
	task, run, agent := tasksIDs()
	path := "/v1/tasks/" + string(task)
	runPath := path + "/runs/" + string(run)
	ex := func(opID string, status int) string { return testkit.Example(opID, status) }
	testkit.Run(t, []testkit.Case{
		{
			Name: "listTasks",
			Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks", NoHeader: tasksNoUser,
				Query: url.Values{"agentId": {string(agent)}, "userId": {"user_42"}, "cursor": {"c1"}, "limit": {"25"}}, RespBody: ex("listTasks", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.List(ctx, ba.ListTasksParams{AgentID: &agent, UserID: new("user_42"), Cursor: new("c1"), Limit: new(25)})
			},
			Check: tasksCovers(ex("listTasks", 200)),
		},
		{
			Name:      "listTasks with no filter",
			Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks", NoHeader: tasksNoUser, RespBody: ex("listTasks", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.List(ctx, ba.ListTasksParams{}) },
			Check: func(t *testing.T, got any) {
				p := got.(*ba.Page[ba.TaskListItem])
				if len(p.Data) != 1 || p.NextCursor != "" {
					t.Fatalf("page %+v", p)
				}
				it := p.Data[0]
				if it.LatestRun == nil || it.LatestRun.Status != ba.TaskRunStatusSucceeded || it.LatestRun.FinishedAt == nil || it.LatestRunID == nil || it.ActiveRunID != nil || it.NextFireAt != nil {
					t.Fatalf("item %+v", it)
				}
			},
		},
		{
			Name: "createTask",
			Exchanges: []testkit.Exchange{{Op: "createTask", Path: "/v1/tasks", NoHeader: tasksNoUser, Status: 201, RespBody: ex("createTask", 201),
				Body: `{"idempotencyKey":"k","agentId":"` + string(agent) + `","name":"n","prompt":"p","schedule":{"kind":"once","config":{"at":"2026-07-13T08:00:00Z"}},"enabled":false,"userId":"user_42","metadata":{"team":"support"}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				at := time.Date(2026, 7, 13, 9, 0, 0, 0, time.FixedZone("x", 3600))
				return c.Tasks.Create(ctx, ba.CreateTaskParams{IdempotencyKey: new("k"), AgentID: agent, Name: "n", Prompt: "p", Enabled: new(false), UserID: new("user_42"),
					Metadata: ba.Metadata{"team": "support"}, Schedule: ba.Value[ba.Schedule](ba.ScheduleOnce{At: at})})
			},
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("createTask", 201))(t, got)
				created := got.(*ba.CreatedTask)
				if created.RunID != nil || created.Task.Schedule == nil || created.Task.NextFireAt == nil || created.Task.DeletedAt != nil {
					t.Fatalf("created %+v", created)
				}
			},
		},
		{
			Name: "createTask submitted",
			Exchanges: []testkit.Exchange{{Op: "createTask", Path: "/v1/tasks", NoHeader: tasksNoUser, Status: 202, RespBody: ex("createTask", 202),
				Body: `{"agentId":"` + string(agent) + `","name":"n","prompt":"p","submit":true}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Create(ctx, ba.CreateTaskParams{AgentID: agent, Name: "n", Prompt: "p", Submit: new(true)})
			},
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("createTask", 202))(t, got)
				created := got.(*ba.CreatedTask)
				if created.RunID == nil || created.Task.ActiveRunID == nil || *created.RunID != *created.Task.ActiveRunID || created.Task.Schedule != nil {
					t.Fatalf("created %+v", created)
				}
			},
		},
		{
			Name:      "getTask",
			Exchanges: []testkit.Exchange{{Op: "getTask", Path: path, NoHeader: tasksNoUser, RespBody: ex("getTask", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.Get(ctx, task) },
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("getTask", 200))(t, got)
				task := got.(*ba.Task)
				cron, ok := task.Schedule.(ba.ScheduleCron)
				if !ok || cron.Expression != "0 9 * * 1-5" || cron.Timezone == nil || *cron.Timezone != "Europe/London" || cron.Stagger != nil {
					t.Fatalf("schedule %#v", task.Schedule)
				}
				if !task.Enabled || task.Metadata["team"] != "support" || task.UserID != "" || task.NextFireAt == nil || !task.NextFireAt.Equal(time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)) {
					t.Fatalf("task %+v", task)
				}
			},
		},
		{
			Name: "updateTask",
			Exchanges: []testkit.Exchange{{Op: "updateTask", Path: path, NoHeader: tasksNoUser, RespBody: ex("updateTask", 200),
				Body: `{"name":"n2","prompt":"p2","schedule":{"kind":"cron","config":{"expression":"0 9 * * 1","timezone":"UTC","staggerMs":60000}},"enabled":false,"metadata":{}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Update(ctx, task, ba.UpdateTaskParams{Name: new("n2"), Prompt: new("p2"), Enabled: new(false), Metadata: ba.Metadata{},
					Schedule: ba.Value[ba.Schedule](ba.ScheduleCron{Expression: "0 9 * * 1", Timezone: new("UTC"), Stagger: new(time.Minute)})})
			},
			Check: tasksCovers(ex("updateTask", 200)),
		},
		{
			Name:      "deleteTask",
			Exchanges: []testkit.Exchange{{Op: "deleteTask", Path: path, NoHeader: tasksNoUser, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Tasks.Delete(ctx, task) },
		},
		{
			Name:      "createTaskRun",
			Exchanges: []testkit.Exchange{{Op: "createTaskRun", Path: path + "/runs", NoHeader: tasksNoUser, Body: `{"idempotencyKey":"k"}`, Status: 202, RespBody: ex("createTaskRun", 202)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Submit(ctx, task, ba.SubmitTaskParams{IdempotencyKey: new("k")})
			},
			Check: tasksCovers(ex("createTaskRun", 202)),
		},
		{
			Name:      "createTaskRun without a key",
			Exchanges: []testkit.Exchange{{Op: "createTaskRun", Path: path + "/runs", Body: `{}`, Status: 202, RespBody: `{"runId":"` + string(run) + `"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Submit(ctx, task, ba.SubmitTaskParams{})
			},
			Check: func(t *testing.T, got any) {
				if got.(*ba.CreatedTaskRun).RunID != run {
					t.Fatalf("run %+v", got)
				}
			},
		},
		{
			Name: "listTaskRuns",
			Exchanges: []testkit.Exchange{{Op: "listTaskRuns", Path: path + "/runs", NoHeader: tasksNoUser,
				Query: url.Values{"cursor": {"c1"}, "limit": {"10"}}, RespBody: ex("listTaskRuns", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.ListRuns(ctx, task, ba.ListTaskRunsParams{Cursor: new("c1"), Limit: new(10)})
			},
			Check: tasksCovers(ex("listTaskRuns", 200)),
		},
		{
			Name:      "getTaskRun",
			Exchanges: []testkit.Exchange{{Op: "getTaskRun", Path: runPath, NoHeader: tasksNoUser, RespBody: ex("getTaskRun", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.GetRun(ctx, task, run) },
			Check: func(t *testing.T, got any) {
				tasksCovers(ex("getTaskRun", 200))(t, got)
				r := got.(*ba.TaskRun)
				if r.Status != ba.TaskRunStatusSucceeded || r.Error != nil || r.SessionID == nil || r.TurnID == nil || r.StartedAt == nil || r.FinishedAt == nil ||
					r.CancelRequestedAt != nil || r.CanceledAt != nil || r.AgentConfig.Name != "Support Agent" || r.AgentConfig.CompactionReserveTokens != 16384 {
					t.Fatalf("run %+v", r)
				}
			},
		},
		{
			Name: "listTaskRunMessages",
			Exchanges: []testkit.Exchange{{Op: "listTaskRunMessages", Path: runPath + "/messages", NoHeader: tasksNoUser,
				Query: url.Values{"after": {"a1"}, "limit": {"5"}}, RespBody: ex("listTaskRunMessages", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.ListRunMessages(ctx, task, run, ba.ListTaskRunMessagesParams{After: new("a1"), Limit: new(5)})
			},
			Check: func(t *testing.T, got any) {
				testkit.CoversExample(t, ex("listTaskRunMessages", 200), got)
				l := got.(*ba.TaskRunMessageList)
				if len(l.Data) != 1 || l.Data[0].Role != ba.MessageRoleAssistant || l.NextCursor != "" || l.LatestCursor == nil || *l.LatestCursor != "eyJzZXEiOjEyfQ" ||
					l.Status != ba.TaskRunStatusSucceeded || l.Error != nil || l.FinishedAt == nil {
					t.Fatalf("list %+v", l)
				}
			},
		},
		{
			Name:      "cancelTaskRun",
			Exchanges: []testkit.Exchange{{Op: "cancelTaskRun", Path: runPath + "/cancel", NoHeader: tasksNoUser, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Tasks.CancelRun(ctx, task, run) },
		},
	})
}

func TestTasksUserClient(t *testing.T) {
	task, run, _ := tasksIDs()
	m := testkit.New(t)
	user, err := m.Client().ForUser("user_42")
	if err != nil {
		t.Fatal(err)
	}
	header := http.Header{"X-Ba-User-Id": {"user_42"}}
	path := "/v1/tasks/" + string(task)
	m.Expect(testkit.Exchange{Op: "listTasks", Path: "/v1/tasks", Header: header, RespBody: `{"data":[],"nextCursor":null}`})
	m.Expect(testkit.Exchange{Op: "createTaskRun", Path: path + "/runs", Header: header, Body: `{}`, Status: 202, RespBody: `{"runId":"` + string(run) + `"}`})
	m.Expect(testkit.Exchange{Op: "cancelTaskRun", Path: path + "/runs/" + string(run) + "/cancel", Header: header, Status: 204})
	ctx := context.Background()
	if _, err := user.Tasks.List(ctx, ba.ListTasksParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := user.Tasks.Submit(ctx, task, ba.SubmitTaskParams{}); err != nil {
		t.Fatal(err)
	}
	if err := user.Tasks.CancelRun(ctx, task, run); err != nil {
		t.Fatal(err)
	}
}

// TestTasksScheduleNullable covers the three states of the nullable Schedule on create and update.
func TestTasksScheduleNullable(t *testing.T) {
	task, _, agent := tasksIDs()
	path := "/v1/tasks/" + string(task)
	minimal := `{"id":"` + string(task) + `","schedule":null}`
	created := `{"task":` + minimal + `,"runId":null}`
	create := func(schedule ba.Nullable[ba.Schedule], body string) testkit.Case {
		return testkit.Case{
			Exchanges: []testkit.Exchange{{Op: "createTask", Path: "/v1/tasks", Status: 201, Body: body, RespBody: created}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Create(ctx, ba.CreateTaskParams{AgentID: agent, Name: "n", Prompt: "p", Schedule: schedule})
			},
		}
	}
	update := func(schedule ba.Nullable[ba.Schedule], body string) testkit.Case {
		return testkit.Case{
			Exchanges: []testkit.Exchange{{Op: "updateTask", Path: path, Body: body, RespBody: minimal}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Update(ctx, task, ba.UpdateTaskParams{Schedule: schedule})
			},
			Check: func(t *testing.T, got any) {
				if task := got.(*ba.Task); task.Schedule != nil || task.ActiveRunID != nil || task.Metadata != nil {
					t.Fatalf("task %+v", task)
				}
			},
		}
	}
	base := `"agentId":"` + string(agent) + `","name":"n","prompt":"p"`
	interval := ba.Value[ba.Schedule](ba.ScheduleInterval{Every: time.Hour})
	cases := []testkit.Case{
		create(ba.Nullable[ba.Schedule]{}, `{`+base+`}`),
		create(ba.Null[ba.Schedule](), `{`+base+`,"schedule":null}`),
		create(interval, `{`+base+`,"schedule":{"kind":"interval","config":{"everyMs":3600000}}}`),
		update(ba.Nullable[ba.Schedule]{}, `{}`),
		update(ba.Null[ba.Schedule](), `{"schedule":null}`),
		update(interval, `{"schedule":{"kind":"interval","config":{"everyMs":3600000}}}`),
	}
	for i, name := range []string{"create omitted", "create null", "create value", "update omitted", "update null", "update value"} {
		cases[i].Name = name
	}
	testkit.Run(t, cases)
}

func TestTasksDecoding(t *testing.T) {
	task, run, _ := tasksIDs()
	path := "/v1/tasks/" + string(task)
	testkit.Run(t, []testkit.Case{
		{
			Name: "list item without a latest run",
			Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks",
				RespBody: `{"data":[{"id":"` + string(task) + `","schedule":null,"latestRun":null,"activeRunId":"` + string(run) + `","deletedAt":"2026-07-10T10:00:00Z"}],"nextCursor":"c2"}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.List(ctx, ba.ListTasksParams{}) },
			Check: func(t *testing.T, got any) {
				p := got.(*ba.Page[ba.TaskListItem])
				it := p.Data[0]
				if p.NextCursor != "c2" || it.LatestRun != nil || it.Schedule != nil || it.ActiveRunID == nil || *it.ActiveRunID != run || it.DeletedAt == nil {
					t.Fatalf("item %+v", it)
				}
			},
		},
		{
			Name: "run with an unknown status and an error",
			Exchanges: []testkit.Exchange{{Op: "getTaskRun", Path: path + "/runs/" + string(run),
				RespBody: `{"id":"` + string(run) + `","status":"paused","error":"boom","sessionId":null,"turnId":null,"agentConfig":{"name":"a","tools":["read","teleport"]}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.GetRun(ctx, task, run) },
			Check: func(t *testing.T, got any) {
				r := got.(*ba.TaskRun)
				if r.Status != ba.TaskRunStatus("paused") || r.Error == nil || *r.Error != "boom" || r.SessionID != nil || r.TurnID != nil || r.AgentConfig.Tools[1] != ba.AgentTool("teleport") {
					t.Fatalf("run %+v", r)
				}
			},
		},
		{
			Name: "message list of a failed run",
			Exchanges: []testkit.Exchange{{Op: "listTaskRunMessages", Path: path + "/runs/" + string(run) + "/messages",
				RespBody: `{"data":[],"nextCursor":null,"latestCursor":null,"status":"failed","error":"Model unavailable","finishedAt":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.ListRunMessages(ctx, task, run, ba.ListTaskRunMessagesParams{})
			},
			Check: func(t *testing.T, got any) {
				l := got.(*ba.TaskRunMessageList)
				if len(l.Data) != 0 || l.NextCursor != "" || l.LatestCursor != nil || l.Status != ba.TaskRunStatusFailed || l.Error == nil || *l.Error != "Model unavailable" || l.FinishedAt != nil {
					t.Fatalf("list %+v", l)
				}
			},
		},
	})
}

// tasksCollect drains a sequence twice and returns the ids seen, proving it restarts.
func tasksCollect[T any](t *testing.T, seq func(func(T, error) bool), id func(T) string) []string {
	t.Helper()
	var first, second []string
	for pass, into := range []*[]string{&first, &second} {
		for v, err := range seq {
			if err != nil {
				t.Fatalf("pass %d: %v", pass, err)
			}
			*into = append(*into, id(v))
		}
	}
	if len(first) != len(second) {
		t.Fatalf("passes differ: %v %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("passes differ: %v %v", first, second)
		}
	}
	return first
}

func TestTasksAll(t *testing.T) {
	task, run, _ := tasksIDs()
	path := "/v1/tasks/" + string(task)
	runPath := path + "/runs/" + string(run)
	item := func(id string) string { return `{"id":"` + id + `","schedule":null}` }
	runItem := func(id string) string { return `{"id":"` + id + `","agentConfig":{"name":"a"}}` }
	message := func(id string) string {
		return `{"id":"` + id + `","role":"assistant","branchable":false,"parts":[{"type":"text","text":"hi"}]}`
	}
	twice := func(pages ...testkit.Exchange) []testkit.Exchange {
		return append(append([]testkit.Exchange{}, pages...), pages...)
	}
	t.Run("tasks", func(t *testing.T) {
		m := testkit.New(t)
		for _, e := range twice(
			testkit.Exchange{Op: "listTasks", Path: "/v1/tasks", Query: url.Values{"limit": {"2"}}, RespBody: `{"data":[` + item("t1") + `,` + item("t2") + `],"nextCursor":"c2"}`},
			testkit.Exchange{Op: "listTasks", Path: "/v1/tasks", Query: url.Values{"limit": {"2"}, "cursor": {"c2"}}, RespBody: `{"data":[` + item("t3") + `],"nextCursor":null}`},
		) {
			m.Expect(e)
		}
		seq := m.Client().Tasks.All(context.Background(), ba.ListTasksParams{Limit: new(2)})
		if got := tasksCollect(t, seq, func(it ba.TaskListItem) string { return string(it.ID) }); len(got) != 3 || got[2] != "t3" {
			t.Fatalf("items %v", got)
		}
	})
	t.Run("tasks from a cursor", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "listTasks", Path: "/v1/tasks", Query: url.Values{"cursor": {"c5"}}, RespBody: `{"data":[` + item("t6") + `],"nextCursor":""}`})
		seq := m.Client().Tasks.All(context.Background(), ba.ListTasksParams{Cursor: new("c5")})
		for it, err := range seq {
			if err != nil || it.ID != "t6" {
				t.Fatalf("%v %v", it, err)
			}
		}
	})
	t.Run("tasks error", func(t *testing.T) {
		m := testkit.New(t)
		m.Expect(testkit.Exchange{Op: "listTasks", Path: "/v1/tasks", RespBody: `{"data":[` + item("t1") + `],"nextCursor":"c2"}`})
		m.Expect(testkit.Exchange{Op: "listTasks", Path: "/v1/tasks", Query: url.Values{"cursor": {"c2"}}, Status: 400, RespBody: `{"error":{"code":"invalid_cursor","message":"Bad cursor"}}`})
		var n int
		var last error
		for _, err := range m.Client().Tasks.All(context.Background(), ba.ListTasksParams{}) {
			n++
			last = err
		}
		if n != 2 {
			t.Fatalf("yielded %d times", n)
		}
		tasksAPIError(t, last, "listTasks", 400, ba.ErrorCodeInvalidCursor, "Bad cursor")
	})
	t.Run("runs", func(t *testing.T) {
		m := testkit.New(t)
		for _, e := range twice(
			testkit.Exchange{Op: "listTaskRuns", Path: path + "/runs", RespBody: `{"data":[` + runItem("r1") + `],"nextCursor":"c2"}`},
			testkit.Exchange{Op: "listTaskRuns", Path: path + "/runs", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[` + runItem("r2") + `],"nextCursor":null}`},
		) {
			m.Expect(e)
		}
		seq := m.Client().Tasks.AllRuns(context.Background(), task, ba.ListTaskRunsParams{})
		if got := tasksCollect(t, seq, func(r ba.TaskRun) string { return string(r.ID) }); len(got) != 2 || got[1] != "r2" {
			t.Fatalf("runs %v", got)
		}
	})
	t.Run("messages by cursor", func(t *testing.T) {
		m := testkit.New(t)
		for _, e := range twice(
			testkit.Exchange{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"limit": {"1"}}, RespBody: `{"data":[` + message("m1") + `],"nextCursor":"c2","status":"running"}`},
			testkit.Exchange{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"limit": {"1"}, "cursor": {"c2"}}, RespBody: `{"data":[` + message("m2") + `],"nextCursor":null,"status":"running"}`},
		) {
			m.Expect(e)
		}
		seq := m.Client().Tasks.AllRunMessages(context.Background(), task, run, ba.ListTaskRunMessagesParams{Limit: new(1)})
		if got := tasksCollect(t, seq, func(msg ba.Message) string { return msg.ID }); len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
			t.Fatalf("messages %v", got)
		}
	})
	t.Run("messages after", func(t *testing.T) {
		m := testkit.New(t)
		for _, e := range twice(
			testkit.Exchange{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"after": {"a1"}}, RespBody: `{"data":[` + message("m2") + `],"nextCursor":"a2","status":"running"}`},
			testkit.Exchange{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"after": {"a2"}}, RespBody: `{"data":[],"nextCursor":null,"status":"succeeded"}`},
		) {
			m.Expect(e)
		}
		seq := m.Client().Tasks.AllRunMessages(context.Background(), task, run, ba.ListTaskRunMessagesParams{After: new("a1")})
		if got := tasksCollect(t, seq, func(msg ba.Message) string { return msg.ID }); len(got) != 1 || got[0] != "m2" {
			t.Fatalf("messages %v", got)
		}
	})
}

// TestTasksCursorAfterExclusive is contract rule 7.4 item 4: nothing is sent when both are set.
func TestTasksCursorAfterExclusive(t *testing.T) {
	task, run, _ := tasksIDs()
	m := testkit.New(t)
	ctx := context.Background()
	params := ba.ListTaskRunMessagesParams{Cursor: new("c1"), After: new("a1")}
	check := func(err error) {
		t.Helper()
		var e *ba.ArgumentError
		if !errors.As(err, &e) || e.Name != "params.Cursor" || e.Err != nil {
			t.Fatalf("error %T %v", err, err)
		}
		if err.Error() != "blazingagents: invalid argument params.Cursor: set together with params.After; use one" {
			t.Fatalf("message %q", err)
		}
	}
	list, err := m.Client().Tasks.ListRunMessages(ctx, task, run, params)
	if list != nil {
		t.Fatalf("result %+v", list)
	}
	check(err)
	var n int
	for msg, err := range m.Client().Tasks.AllRunMessages(ctx, task, run, params) {
		n++
		if msg.ID != "" {
			t.Fatalf("message %+v", msg)
		}
		check(err)
	}
	if n != 1 {
		t.Fatalf("yielded %d times", n)
	}
}

func TestTasksErrors(t *testing.T) {
	task, run, agent := tasksIDs()
	path := "/v1/tasks/" + string(task)
	runPath := path + "/runs/" + string(run)
	body := func(code ba.ErrorCode, message string) string {
		return `{"error":{"code":"` + string(code) + `","message":"` + message + `"}}`
	}
	ctx := context.Background()
	for _, tc := range []struct {
		op      string
		path    string
		query   url.Values
		body    string
		status  int
		code    ba.ErrorCode
		message string
		call    func(c *ba.Client) (any, error)
	}{
		{"listTasks", "/v1/tasks", url.Values{"limit": {"1000"}}, "", 400, ba.ErrorCodeValidationFailed, "limit must be at most 100", func(c *ba.Client) (any, error) {
			return c.Tasks.List(ctx, ba.ListTasksParams{Limit: new(1000)})
		}},
		{"createTask", "/v1/tasks", nil, `{"agentId":"` + string(agent) + `","name":"n","prompt":"p"}`, 404, ba.ErrorCodeNotFound, "Agent not found", func(c *ba.Client) (any, error) {
			return c.Tasks.Create(ctx, ba.CreateTaskParams{AgentID: agent, Name: "n", Prompt: "p"})
		}},
		{"getTask", path, nil, "", 404, ba.ErrorCodeNotFound, "Task not found", func(c *ba.Client) (any, error) { return c.Tasks.Get(ctx, task) }},
		{"updateTask", path, nil, `{"name":"n"}`, 400, ba.ErrorCodeValidationFailed, "name is too long", func(c *ba.Client) (any, error) {
			return c.Tasks.Update(ctx, task, ba.UpdateTaskParams{Name: new("n")})
		}},
		{"deleteTask", path, nil, "", 409, ba.ErrorCodeTaskActiveRunExists, "A run is active", func(c *ba.Client) (any, error) { return nil, c.Tasks.Delete(ctx, task) }},
		{"createTaskRun", path + "/runs", nil, `{}`, 409, ba.ErrorCodeTaskActiveRunExists, "A run is active", func(c *ba.Client) (any, error) {
			return c.Tasks.Submit(ctx, task, ba.SubmitTaskParams{})
		}},
		{"listTaskRuns", path + "/runs", nil, "", 404, ba.ErrorCodeNotFound, "Task not found", func(c *ba.Client) (any, error) {
			return c.Tasks.ListRuns(ctx, task, ba.ListTaskRunsParams{})
		}},
		{"getTaskRun", runPath, nil, "", 404, ba.ErrorCodeNotFound, "Run not found", func(c *ba.Client) (any, error) { return c.Tasks.GetRun(ctx, task, run) }},
		{"listTaskRunMessages", runPath + "/messages", url.Values{"cursor": {"x"}}, "", 400, ba.ErrorCodeInvalidCursor, "Bad cursor", func(c *ba.Client) (any, error) {
			return c.Tasks.ListRunMessages(ctx, task, run, ba.ListTaskRunMessagesParams{Cursor: new("x")})
		}},
		{"cancelTaskRun", runPath + "/cancel", nil, "", 404, ba.ErrorCodeNotFound, "Run not found", func(c *ba.Client) (any, error) {
			return nil, c.Tasks.CancelRun(ctx, task, run)
		}},
	} {
		t.Run(tc.op, func(t *testing.T) {
			m := testkit.New(t)
			m.Expect(testkit.Exchange{Op: tc.op, Path: tc.path, Query: tc.query, Body: tc.body, Status: tc.status, RespBody: body(tc.code, tc.message)})
			got, err := tc.call(m.Client())
			if got != nil && !reflect.ValueOf(got).IsNil() {
				t.Fatalf("result %#v, want nil", got)
			}
			tasksAPIError(t, err, tc.op, tc.status, tc.code, tc.message)
		})
	}
}

func TestTasksSchedule(t *testing.T) {
	task, _, _ := tasksIDs()
	path := "/v1/tasks/" + string(task)
	body := func(schedule string) string { return `{"id":"` + string(task) + `","schedule":` + schedule + `}` }
	get := func(schedule string) testkit.Exchange {
		return testkit.Exchange{Op: "getTask", Path: path, RespBody: body(schedule)}
	}
	call := func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.Get(ctx, task) }
	schedule := func(want ba.Schedule) func(*testing.T, any) {
		return func(t *testing.T, got any) {
			if s := got.(*ba.Task).Schedule; jsonString(s) != jsonString(want) {
				t.Fatalf("schedule %#v, want %#v", s, want)
			}
		}
	}
	at := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	testkit.Run(t, []testkit.Case{
		{Name: "once", Exchanges: []testkit.Exchange{get(`{"kind":"once","config":{"at":"2026-07-13T08:00:00Z"}}`)}, Call: call, Check: schedule(ba.ScheduleOnce{At: at})},
		{Name: "interval", Exchanges: []testkit.Exchange{get(`{"kind":"interval","config":{"everyMs":90000}}`)}, Call: call, Check: schedule(ba.ScheduleInterval{Every: 90 * time.Second})},
		{Name: "cron", Exchanges: []testkit.Exchange{get(`{"kind":"cron","config":{"expression":"* * * * *","staggerMs":1000}}`)}, Call: call,
			Check: schedule(ba.ScheduleCron{Expression: "* * * * *", Stagger: new(time.Second)})},
		{Name: "unknown", Exchanges: []testkit.Exchange{get(`{"kind":"sunset","config":{"offsetMs":5},"extra":[1]}`)}, Call: call, Unknown: true,
			Check: func(t *testing.T, got any) {
				if u, ok := got.(*ba.Task).Schedule.(ba.UnknownSchedule); !ok || u.Kind != "sunset" {
					t.Fatalf("schedule %#v", got.(*ba.Task).Schedule)
				}
			}},
		{Name: "missing required key", Exchanges: []testkit.Exchange{get(`{"kind":"once","config":{}}`)}, Call: call,
			Err: `blazingagents: getTask: decode response: Schedule once: missing "at"`},
		{Name: "missing payload", Exchanges: []testkit.Exchange{get(`{"kind":"cron"}`)}, Call: call,
			Err: `blazingagents: getTask: decode response: Schedule cron: missing "config"`},
		{Name: "payload of the wrong type", Exchanges: []testkit.Exchange{get(`{"kind":"interval","config":{"everyMs":"soon"}}`)}, Call: call,
			ErrPrefix: "blazingagents: getTask: decode response: json: "},
		{Name: "host field of the wrong type", Exchanges: []testkit.Exchange{{Op: "getTask", Path: path, RespBody: `{"id":7}`}}, Call: call,
			ErrPrefix: "blazingagents: getTask: decode response: json: "},
		{Name: "list host field of the wrong type", Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks", RespBody: `{"data":[{"id":7}]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.List(ctx, ba.ListTasksParams{}) },
			ErrPrefix: "blazingagents: listTasks: decode response: json: "},
		{Name: "list unknown", Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks", RespBody: `{"data":[{"id":"` + string(task) + `","schedule":{"kind":"lunar"}}],"nextCursor":null}`}},
			Call:    func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.List(ctx, ba.ListTasksParams{}) },
			Unknown: true},
		{Name: "list missing required key", Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks", RespBody: `{"data":[{"id":"` + string(task) + `","schedule":{"kind":"interval","config":{}}}],"nextCursor":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.List(ctx, ba.ListTasksParams{}) },
			Err:  `blazingagents: listTasks: decode response: Schedule interval: missing "everyMs"`},
	})
}

func TestTasksScheduleEncoding(t *testing.T) {
	task, _, _ := tasksIDs()
	m := testkit.New(t)
	ctx := context.Background()
	_, err := m.Client().Tasks.Update(ctx, task, ba.UpdateTaskParams{Schedule: ba.Value[ba.Schedule](ba.ScheduleOnce{})})
	if err == nil || err.Error() != "blazingagents: invalid argument ScheduleOnce.At: zero" {
		t.Fatalf("error %v, want *ArgumentError about the zero At", err)
	}
	unknown := ba.UnknownSchedule{Kind: "lunar", Raw: []byte(`{"kind":"lunar","phase":"full"}`)}
	m.Expect(testkit.Exchange{Op: "updateTask", Path: "/v1/tasks/" + string(task), Body: `{"schedule":{"kind":"lunar","phase":"full"}}`, RespBody: `{"id":"` + string(task) + `","schedule":null}`})
	if _, err := m.Client().Tasks.Update(ctx, task, ba.UpdateTaskParams{Schedule: ba.Value[ba.Schedule](unknown)}); err != nil {
		t.Fatal(err)
	}
}
