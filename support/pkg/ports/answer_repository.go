package ports

import "context"

// AnswerRepository stores the copy each menu node opens with.
//
// Keyed by node and language so staff can edit wording without a deploy. Only
// Korean rows ship at launch; the language is in the key from day one because
// this repo migrates schema inline and additively, and widening a primary key
// later is exactly the breaking change that convention does not cover.
type AnswerRepository interface {
	// Body returns the stored copy, or "" when the node has none.
	Body(ctx context.Context, node, language string) (string, error)
	// All returns every node's copy for a language, for the editor.
	All(ctx context.Context, language string) (map[string]string, error)
	// Put replaces one node's copy, recording who changed it.
	Put(ctx context.Context, node, language, body, updatedBy string) error
}
