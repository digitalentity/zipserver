package notifications

import (
	"strings"

	"zipserver/internal/comments"
)

// GetNotificationRecipients calculates the list of unique email addresses to notify
// for a new comment. It excludes the creator of the new comment.
func GetNotificationRecipients(newAnno comments.Annotation, pageComments []comments.Annotation, globalWatchers []string) []string {
	recipientsMap := make(map[string]bool)

	// 1. Add global watchers
	for _, email := range globalWatchers {
		cleaned := strings.TrimSpace(strings.ToLower(email))
		if cleaned != "" {
			recipientsMap[cleaned] = true
		}
	}

	// 2. Add thread participants if it is a reply
	if newAnno.Motivation == "replying" {
		parentID := newAnno.Target.String
		if parentID != "" {
			var parentCreatorEmail string
			// Find the parent comment to notify its author
			for _, anno := range pageComments {
				if anno.ID == parentID {
					parentCreatorEmail = strings.TrimSpace(strings.ToLower(anno.Creator.Email))
					break
				}
			}
			if parentCreatorEmail != "" {
				recipientsMap[parentCreatorEmail] = true
			}

			// Find other participants in the thread (other replies)
			for _, anno := range pageComments {
				if anno.Motivation == "replying" && anno.Target.String == parentID {
					participantEmail := strings.TrimSpace(strings.ToLower(anno.Creator.Email))
					if participantEmail != "" {
						recipientsMap[participantEmail] = true
					}
				}
			}
		}
	}

	// 3. Remove the creator of the new comment from the notification list
	creatorEmail := strings.TrimSpace(strings.ToLower(newAnno.Creator.Email))
	if creatorEmail != "" {
		delete(recipientsMap, creatorEmail)
	}

	var list []string
	for email := range recipientsMap {
		list = append(list, email)
	}
	return list
}
