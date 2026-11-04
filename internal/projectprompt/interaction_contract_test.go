package projectprompt

import (
	"strings"
	"testing"
)

func TestUserInteractionContractSectionEnglishTurn(t *testing.T) {
	t.Parallel()
	got := UserInteractionContractSection("Please verify the DNS TXT record first.")
	for _, want := range []string{
		"Do not answer in Chinese",
		"default to English",
		"live DNS lookup",
		"WHOIS result is not a substitute",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("contract missing %q: %s", want, got)
		}
	}
}

func TestUserInteractionContractSectionHanTurn(t *testing.T) {
	t.Parallel()
	got := UserInteractionContractSection("请先验证 DNS TXT 记录。")
	if !strings.Contains(got, "contains Han-script text") {
		t.Fatalf("expected Han-script language hint: %s", got)
	}
	if strings.Contains(got, "Do not answer in Chinese") {
		t.Fatalf("Chinese turn must not receive the non-Chinese hint: %s", got)
	}
}
