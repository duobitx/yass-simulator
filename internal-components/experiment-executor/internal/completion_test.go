package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// newCompletionApp builds an AppType wired only for the server-side completion
// path, with the given per-node terminator rules.
func newCompletionApp(terminators map[string]completionRule) *AppType {
	return &AppType{
		terminators:    terminators,
		produced:       map[string]struct{}{},
		receivedByNode: map[string]map[string]struct{}{},
	}
}

func put(name string) *crudEventMsg { return &crudEventMsg{Type: "PUT", Name: name} }
func recv(gs, name string) *crudEventMsg {
	return &crudEventMsg{Type: "RECEIVED", FsNodeName: gs, Name: name}
}

func TestCompletionFirstFileAtAnyTerminator(t *testing.T) {
	app := newCompletionApp(map[string]completionRule{"gs-a": {n: 1}, "gs-b": {n: 1}})

	app.recordCrudForCompletion(put("photo-0"))
	done, _ := app.recordCrudForCompletion(recv("sat-relay", "photo-0")) // not a terminator
	assert.False(t, done)

	done, comment := app.recordCrudForCompletion(recv("gs-b", "photo-0"))
	assert.True(t, done)
	assert.Contains(t, comment, "gs-b")
}

func TestCompletionAllFilesAtTerminator(t *testing.T) {
	app := newCompletionApp(map[string]completionRule{"gs-a": {countAll: true}})

	app.recordCrudForCompletion(put("a"))
	app.recordCrudForCompletion(put("b"))

	done, _ := app.recordCrudForCompletion(recv("gs-a", "a"))
	assert.False(t, done) // only 1 of 2 produced files

	done, comment := app.recordCrudForCompletion(recv("gs-a", "b"))
	assert.True(t, done)
	assert.Contains(t, comment, "all 2")
}

func TestCompletionFixedCount(t *testing.T) {
	app := newCompletionApp(map[string]completionRule{"gs-a": {n: 2}})

	done, _ := app.recordCrudForCompletion(recv("gs-a", "a"))
	assert.False(t, done)
	done, _ = app.recordCrudForCompletion(recv("gs-a", "a")) // duplicate name — still 1 unique
	assert.False(t, done)
	done, _ = app.recordCrudForCompletion(recv("gs-a", "b"))
	assert.True(t, done)
}

func TestEnvTrue(t *testing.T) {
	for _, v := range []string{"true", "True", "1", "yes", "on", " t "} {
		assert.True(t, envTrue(v), v)
	}
	for _, v := range []string{"", "false", "0", "no"} {
		assert.False(t, envTrue(v), v)
	}
}
