package blazingagents_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

func tasksIDs() (ba.TaskID, ba.TaskRunID, ba.AgentID) {
	return ba.TaskID(testkit.ID(op.IDPatterns["TaskID"])), ba.TaskRunID(testkit.ID(op.IDPatterns["TaskRunID"])), ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
}

func TestTasksSmoke(t *testing.T) {
	task, run, agent := tasksIDs()
	path := "/v1/tasks/" + string(task)
	runPath := path + "/runs/" + string(run)
	item := `{"id":"` + string(task) + `","schedule":null}`
	runItem := `{"id":"` + string(run) + `","agentConfig":{"name":"a","model":null,"thinkingLevel":null,"providerId":null,"autoCompaction":false,"compactionReserveTokens":0,"memoryInjectionEnabled":false,"tools":[],"instructions":"","metadata":{},"mcpConnectionIds":[]}}`
	at := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	messages := `{"data":[{"id":"m1","role":"assistant","branchable":false,"parts":[]}],"nextCursor":"c2"}`
	collect := func(seq func(func(ba.Message, error) bool)) (any, error) {
		var ids []string
		for m, err := range seq {
			if err != nil {
				return nil, err
			}
			ids = append(ids, m.ID)
		}
		return ids, nil
	}
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listTasks",
			Exchanges: []testkit.Exchange{{Op: "listTasks", Path: "/v1/tasks", Query: url.Values{"agentId": {string(agent)}}, RespBody: testkit.Example("listTasks", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.List(ctx, ba.ListTasksParams{AgentID: &agent})
			},
			Example: testkit.Example("listTasks", 200),
		},
		{
			Name: "listTasks all",
			Exchanges: []testkit.Exchange{
				{Op: "listTasks", Path: "/v1/tasks", RespBody: `{"data":[` + item + `],"nextCursor":"c2"}`},
				{Op: "listTasks", Path: "/v1/tasks", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[` + item + `],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var n int
				for _, err := range c.Tasks.All(ctx, ba.ListTasksParams{}) {
					if err != nil {
						return nil, err
					}
					n++
				}
				return n, nil
			},
			Example: `2`,
		},
		{
			Name: "createTask",
			Exchanges: []testkit.Exchange{{Op: "createTask", Path: "/v1/tasks", Status: 201, RespBody: testkit.Example("createTask", 201),
				Body: `{"agentId":"` + string(agent) + `","name":"n","prompt":"p","schedule":{"kind":"once","config":{"at":"2026-07-13T08:00:00Z"}}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Create(ctx, ba.CreateTaskParams{AgentID: agent, Name: "n", Prompt: "p", Schedule: ba.Value[ba.Schedule](ba.ScheduleOnce{At: at.In(time.FixedZone("x", 3600))})})
			},
			Example: testkit.Example("createTask", 201),
		},
		{
			Name: "createTask 202",
			Exchanges: []testkit.Exchange{{Op: "createTask", Path: "/v1/tasks", Status: 202, RespBody: testkit.Example("createTask", 202),
				Body: `{"agentId":"` + string(agent) + `","name":"n","prompt":"p","schedule":{"kind":"interval","config":{"everyMs":3600000}},"submit":true}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Create(ctx, ba.CreateTaskParams{AgentID: agent, Name: "n", Prompt: "p", Submit: new(true), Schedule: ba.Value[ba.Schedule](ba.ScheduleInterval{Every: time.Hour})})
			},
			Example: testkit.Example("createTask", 202),
		},
		{
			Name:      "getTask",
			Exchanges: []testkit.Exchange{{Op: "getTask", Path: path, RespBody: testkit.Example("getTask", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.Get(ctx, task) },
			Example:   testkit.Example("getTask", 200),
		},
		{
			Name: "updateTask",
			Exchanges: []testkit.Exchange{{Op: "updateTask", Path: path, RespBody: testkit.Example("updateTask", 200),
				Body: `{"schedule":{"kind":"cron","config":{"expression":"0 9 * * 1","timezone":"UTC","staggerMs":60000}}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Update(ctx, task, ba.UpdateTaskParams{Schedule: ba.Value[ba.Schedule](ba.ScheduleCron{Expression: "0 9 * * 1", Timezone: new("UTC"), Stagger: new(time.Minute)})})
			},
			Example: testkit.Example("updateTask", 200),
		},
		{
			Name:      "updateTask clears schedule",
			Exchanges: []testkit.Exchange{{Op: "updateTask", Path: path, Body: `{"schedule":null}`, RespBody: item}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Update(ctx, task, ba.UpdateTaskParams{Schedule: ba.Null[ba.Schedule]()})
			},
			Example: item,
		},
		{
			Name:      "deleteTask",
			Exchanges: []testkit.Exchange{{Op: "deleteTask", Path: path, Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Tasks.Delete(ctx, task) },
		},
		{
			Name:      "createTaskRun",
			Exchanges: []testkit.Exchange{{Op: "createTaskRun", Path: path + "/runs", Body: `{"idempotencyKey":"k"}`, Status: 202, RespBody: testkit.Example("createTaskRun", 202)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.Submit(ctx, task, ba.SubmitTaskParams{IdempotencyKey: new("k")})
			},
			Example: testkit.Example("createTaskRun", 202),
		},
		{
			Name:      "listTaskRuns",
			Exchanges: []testkit.Exchange{{Op: "listTaskRuns", Path: path + "/runs", RespBody: testkit.Example("listTaskRuns", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.ListRuns(ctx, task, ba.ListTaskRunsParams{})
			},
			Example: testkit.Example("listTaskRuns", 200),
		},
		{
			Name: "listTaskRuns all",
			Exchanges: []testkit.Exchange{
				{Op: "listTaskRuns", Path: path + "/runs", RespBody: `{"data":[` + runItem + `],"nextCursor":"c2"}`},
				{Op: "listTaskRuns", Path: path + "/runs", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var ids []ba.TaskRunID
				for r, err := range c.Tasks.AllRuns(ctx, task, ba.ListTaskRunsParams{}) {
					if err != nil {
						return nil, err
					}
					ids = append(ids, r.ID)
				}
				return ids, nil
			},
			Example: `["` + string(run) + `"]`,
		},
		{
			Name:      "getTaskRun",
			Exchanges: []testkit.Exchange{{Op: "getTaskRun", Path: runPath, RespBody: testkit.Example("getTaskRun", 200)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.Tasks.GetRun(ctx, task, run) },
			Example:   testkit.Example("getTaskRun", 200),
		},
		{
			Name:      "listTaskRunMessages",
			Exchanges: []testkit.Exchange{{Op: "listTaskRunMessages", Path: runPath + "/messages", RespBody: testkit.Example("listTaskRunMessages", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.Tasks.ListRunMessages(ctx, task, run, ba.ListTaskRunMessagesParams{})
			},
			Check: func(t *testing.T, got any) {
				if l := got.(*ba.TaskRunMessageList); len(l.Data) != 1 {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name: "listTaskRunMessages all by cursor",
			Exchanges: []testkit.Exchange{
				{Op: "listTaskRunMessages", Path: runPath + "/messages", RespBody: messages},
				{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return collect(c.Tasks.AllRunMessages(ctx, task, run, ba.ListTaskRunMessagesParams{}))
			},
			Example: `["m1"]`,
		},
		{
			Name: "listTaskRunMessages all after",
			Exchanges: []testkit.Exchange{
				{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"after": {"c1"}}, RespBody: messages},
				{Op: "listTaskRunMessages", Path: runPath + "/messages", Query: url.Values{"after": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return collect(c.Tasks.AllRunMessages(ctx, task, run, ba.ListTaskRunMessagesParams{After: new("c1")}))
			},
			Example: `["m1"]`,
		},
		{
			Name:      "cancelTaskRun",
			Exchanges: []testkit.Exchange{{Op: "cancelTaskRun", Path: runPath + "/cancel", Status: 204}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.Tasks.CancelRun(ctx, task, run) },
		},
	})
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
