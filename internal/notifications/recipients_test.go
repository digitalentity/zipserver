package notifications

import (
	"sort"
	"testing"

	"zipserver/internal/comments"
)

func TestGetNotificationRecipients(t *testing.T) {
	watchers := []string{"admin@example.com", "watch@example.com"}

	// Comments list on page
	pageComments := []comments.Annotation{
		{
			ID: "anno_root",
			Creator: comments.Creator{
				Email: "author@example.com",
			},
		},
		{
			ID:         "reply_1",
			Motivation: "replying",
			Target:     comments.Target{String: "anno_root"},
			Creator: comments.Creator{
				Email: "replier1@example.com",
			},
		},
	}

	tests := []struct {
		name     string
		newAnno  comments.Annotation
		expected []string
	}{
		{
			name: "New root comment - notifies watchers",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
			},
			expected: []string{"admin@example.com", "watch@example.com"},
		},
		{
			name: "New root comment by watcher - excludes creator",
			newAnno: comments.Annotation{
				ID: "new_root_watcher",
				Creator: comments.Creator{
					Email: "admin@example.com",
				},
			},
			expected: []string{"watch@example.com"},
		},
		{
			name: "New reply - notifies parent author, previous repliers, and watchers",
			newAnno: comments.Annotation{
				ID:         "reply_2",
				Motivation: "replying",
				Target:     comments.Target{String: "anno_root"},
				Creator: comments.Creator{
					Email: "replier2@example.com",
				},
			},
			expected: []string{
				"admin@example.com",
				"author@example.com",
				"replier1@example.com",
				"watch@example.com",
			},
		},
		{
			name: "New reply by parent author - excludes creator from notifications",
			newAnno: comments.Annotation{
				ID:         "reply_3",
				Motivation: "replying",
				Target:     comments.Target{String: "anno_root"},
				Creator: comments.Creator{
					Email: "author@example.com",
				},
			},
			expected: []string{
				"admin@example.com",
				"replier1@example.com",
				"watch@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetNotificationRecipients(tt.newAnno, pageComments, watchers)
			sort.Strings(got)
			sort.Strings(tt.expected)

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tt.expected[i], got[i])
				}
			}
		})
	}
}
