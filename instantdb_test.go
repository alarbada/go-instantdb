package idb

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/matryer/is"
)

func TestAuth(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	client := newClient()

	user1, err := client.CreateToken(ctx, "test@test.gmail")
	is.NoErr(err)

	user2, err := client.VerifyToken(ctx, user1.RefreshToken)
	is.NoErr(err)

	is.Equal("", cmp.Diff(user1, user2))
}

type Todo struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	Done  bool   `json:"done,omitempty"`
}

type List struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

func TestTodoApp(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	client := newClient()

	deletePreviousTodos(ctx, is)

	todos := setupTodos(ctx, is, client)

	err := client.Transact(ctx, []Transaction{
		Update{"todos", todos[0].ID, Todo{Done: true}},
		Update{"todos", todos[1].ID, Todo{Title: "todo 2 (updated)"}},
		Delete{"todos", todos[2].ID},
	})
	is.NoErr(err)

	todos[0].Done = true
	todos[1].Title = "todo 2 (updated)"
	todos = todos[:2]

	assertTodos(ctx, is, todos)
}

func TestLinks(t *testing.T) {
	// TODO: might aswell maybe simplify this thing with google-cmp, but this works

	is := is.New(t)
	ctx := context.Background()
	client := newClient()

	deletePreviousLists(ctx, is)
	deletePreviousTodos(ctx, is)

	todoIDs := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	listIDs := []string{uuid.NewString(), uuid.NewString()}

	err := client.Transact(ctx, []Transaction{
		Update{"todos", todoIDs[0], Todo{Title: "Buy groceries", Done: false}},
		Update{"todos", todoIDs[1], Todo{Title: "Do laundry", Done: false}},
		Update{"todos", todoIDs[2], Todo{Title: "Call mom", Done: false}},
		Update{"lists", listIDs[0], List{Title: "Home chores"}},
		Update{"lists", listIDs[1], List{Title: "Personal"}},
	})
	is.NoErr(err)

	err = client.Transact(ctx, []Transaction{
		(&Link{}).From("lists", listIDs[0]).To("todos", todoIDs[0]),
		(&Link{}).From("lists", listIDs[0]).To("todos", todoIDs[1]),
		(&Link{}).From("lists", listIDs[1]).To("todos", todoIDs[2]),
	})
	is.NoErr(err)

	var result struct {
		Lists []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Todos []Todo `json:"todos"`
		} `json:"lists"`
		Todos []Todo `json:"todos"`
	}

	// I believe insert order is not guaranteed, so let's make it deterministic
	sortResultNamespaces := func() {
		sort.Slice(result.Lists, func(i, j int) bool {
			return result.Lists[i].Title < result.Lists[j].Title
		})
		sort.Slice(result.Todos, func(i, j int) bool {
			return result.Todos[i].Title < result.Todos[j].Title
		})
	}

	err = client.Query(ctx, O{
		"lists": O{"todos": struct{}{}},
		"todos": struct{}{},
	}, &result)
	is.NoErr(err)

	sortResultNamespaces()

	// Assert links
	is.Equal(len(result.Lists), 2)
	is.Equal(len(result.Lists[0].Todos), 2)
	is.Equal(len(result.Lists[1].Todos), 1)
	is.Equal(len(result.Todos), 3)

	// Unlink a todo from a list
	err = client.Transact(ctx, []Transaction{
		(&Unlink{}).From("lists", listIDs[0]).To("todos", todoIDs[1]),
	})
	is.NoErr(err)

	err = client.Query(ctx, O{
		"lists": O{"todos": struct{}{}},
		"todos": struct{}{},
	}, &result)
	is.NoErr(err)

	sortResultNamespaces()

	// Assert unlinks
	is.Equal(len(result.Lists), 2)
	is.Equal(len(result.Lists[0].Todos), 1)
	is.Equal(len(result.Lists[1].Todos), 1)
	is.Equal(len(result.Todos), 3)
}

func TestLookup(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	client := newClient()

	deletePreviousTodos(ctx, is)
	todos := setupTodos(ctx, is, client)

	updatedTodo := &todos[0]
	updatedTodo.Done = !updatedTodo.Done

	err := client.Transact(ctx, []Transaction{
		Update{"todos", Lookup("title", todos[0].Title), Todo{Done: updatedTodo.Done}},
	})
	is.NoErr(err)

	assertTodos(ctx, is, todos)
}

func assertTodos(ctx context.Context, is *is.I, expected []Todo) {
	client := newClient()
	var savedTodos struct {
		Todos []Todo
	}

	err := client.Query(ctx, O{"todos": struct{}{}}, &savedTodos)
	is.NoErr(err)

	sort.Slice(savedTodos.Todos, func(i, j int) bool {
		return savedTodos.Todos[i].Title < savedTodos.Todos[j].Title
	})

	for i := range expected {
		is.Equal("", cmp.Diff(expected[i], savedTodos.Todos[i]))
	}
}

func TestMain(m *testing.M) {
	godotenv.Load()
	m.Run()
}

func newClient() *Client {
	return NewClient(
		os.Getenv("APP_ID"),
		os.Getenv("SECRET"),
	)
}

func deletePreviousTodos(ctx context.Context, is *is.I) {
	client := newClient()
	var savedTodos struct {
		Todos []Todo
	}

	err := client.Query(ctx, O{"todos": struct{}{}}, &savedTodos)
	is.NoErr(err)
	if len(savedTodos.Todos) == 0 {
		return
	}

	var transactions []Transaction
	for _, todo := range savedTodos.Todos {
		transactions = append(transactions, Delete{"todos", todo.ID})
	}

	is.NoErr(client.Transact(ctx, transactions))
}

func deletePreviousLists(ctx context.Context, is *is.I) {
	client := newClient()
	var savedLists struct {
		Lists []List
	}

	err := client.Query(ctx, O{"lists": struct{}{}}, &savedLists)
	is.NoErr(err)
	if len(savedLists.Lists) == 0 {
		return
	}

	var transactions []Transaction
	for _, list := range savedLists.Lists {
		transactions = append(transactions, Delete{"lists", list.ID})
	}

	is.NoErr(client.Transact(ctx, transactions))
}

func setupTodos(ctx context.Context, is *is.I, client *Client) []Todo {
	todos := []Todo{
		{ID: uuid.NewString(), Title: "todo 1", Done: false},
		{ID: uuid.NewString(), Title: "todo 2", Done: false},
		{ID: uuid.NewString(), Title: "todo 3", Done: false},
	}

	err := client.Transact(ctx, []Transaction{
		Update{"todos", todos[0].ID, todos[0]},
		Update{"todos", todos[1].ID, todos[1]},
		Update{"todos", todos[2].ID, todos[2]},
	})
	is.NoErr(err)

	return todos
}
