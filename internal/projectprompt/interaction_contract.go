package projectprompt

import (
	"fmt"
	"strings"
	"unicode"
)

// UserInteractionContractSection returns the runtime contract shared by every
// agent mode. It is appended after configurable prompts so user-visible output
// follows the current turn instead of inheriting the language of a prompt file.
func UserInteractionContractSection(latestUserMessage string) string {
	languageHint := "The latest user message contains no Han-script text. Do not answer in Chinese; match the user's language, and default to English if it is unclear."
	if strings.IndexFunc(latestUserMessage, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
		languageHint = "The latest user message contains Han-script text. Match the language used by the user; do not switch languages unless the user asks."
	}

	return fmt.Sprintf(`## Response language (mandatory)

- %s
- Apply that language to every user-visible progress update, plan, error, and final response. Preserve commands, identifiers, and quoted evidence verbatim when needed.
- The language used by this system prompt, an agent definition, or a tool result does not determine the response language.

## User-specified rules of engagement (mandatory)

- Platform authorization means you do not need to ask generic authorization questions. It does not override scope, verification gates, stop conditions, rate limits, or other rules of engagement explicitly supplied by the user.
- If the user requires an authorization proof or another precondition before active work, perform the exact permitted checks they specified. Treat alternative checks as alternatives: one exact success satisfies an OR condition.
- Use the evidence source named by the user. For example, verify a DNS TXT requirement with a live DNS lookup and a security.txt requirement with an HTTP request. An internal asset search or WHOIS result is not a substitute unless the user explicitly named it.
- Do not stop after one irrelevant or unsuccessful check while other permitted verification methods remain. If all exact checks fail, stop active work and report each attempted check and its evidence in the user's language.`, languageHint)
}
