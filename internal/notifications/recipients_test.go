package notifications

import (
	"sort"
	"strings"
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

	isAllowedUser := func(email string) bool {
		return strings.HasSuffix(email, "@example.com")
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
		{
			name: "New root comment with valid mention - notifies watchers and mentioned user",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
				Body: comments.TextualBody{
					Value: "Hey @mentioned@example.com could you check this?",
				},
			},
			expected: []string{"admin@example.com", "watch@example.com", "mentioned@example.com"},
		},
		{
			name: "New root comment with duplicate mentions - notifies mentioned user only once",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
				Body: comments.TextualBody{
					Value: "Hey @mentioned@example.com and again @mentioned@example.com",
				},
			},
			expected: []string{"admin@example.com", "watch@example.com", "mentioned@example.com"},
		},
		{
			name: "New root comment with self mention - excludes creator from notifications",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
				Body: comments.TextualBody{
					Value: "Self mention here @newuser@example.com",
				},
			},
			expected: []string{"admin@example.com", "watch@example.com"},
		},
		{
			name: "New root comment with unauthorized mention - filters out unauthorized email",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
				Body: comments.TextualBody{
					Value: "Check this out @spammer@blocked.com and @mentioned@example.com",
				},
			},
			expected: []string{"admin@example.com", "watch@example.com", "mentioned@example.com"},
		},
		{
			name: "New root comment with over 10 mentions - caps at 10 unique mentions",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
				Body: comments.TextualBody{
					Value: "Hey @m1@example.com @m2@example.com @m3@example.com @m4@example.com @m5@example.com @m6@example.com @m7@example.com @m8@example.com @m9@example.com @m10@example.com @m11@example.com @m12@example.com",
				},
			},
			expected: []string{
				"admin@example.com", "watch@example.com",
				"m1@example.com", "m2@example.com", "m3@example.com", "m4@example.com", "m5@example.com",
				"m6@example.com", "m7@example.com", "m8@example.com", "m9@example.com", "m10@example.com",
			},
		},
		{
			name: "Mentions inside normal text words are ignored",
			newAnno: comments.Annotation{
				ID: "new_root",
				Creator: comments.Creator{
					Email: "newuser@example.com",
				},
				Body: comments.TextualBody{
					Value: "Contact support at help@example.com or user@example.com@invalid.com or text@abc@example.com",
				},
			},
			expected: []string{"admin@example.com", "watch@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetNotificationRecipients(tt.newAnno, pageComments, watchers, isAllowedUser)
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

