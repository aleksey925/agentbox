package agents

import "testing"

func TestCursorVersionRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "current hyphenated version",
			input: `DOWNLOAD_URL="https://downloads.cursor.com/lab/2026.06.19-20-24-33-653a7fb/${OS}/${ARCH}/agent-cli-package.tar.gz"`,
			want:  "2026.06.19-20-24-33-653a7fb",
		},
		{
			name: "no trailing dash bleed from sibling tmp path",
			input: `TEMP_EXTRACT_DIR=".tmp-2026.06.19-20-24-33-653a7fb-$(date +%s)"
DOWNLOAD_URL="https://downloads.cursor.com/lab/2026.06.19-20-24-33-653a7fb/${OS}/${ARCH}/agent-cli-package.tar.gz"`,
			want: "2026.06.19-20-24-33-653a7fb",
		},
		{
			name:  "legacy format without time fields",
			input: `DOWNLOAD_URL="https://downloads.cursor.com/lab/2025.01.02-abcdef0/linux/x64/agent-cli-package.tar.gz"`,
			want:  "2025.01.02-abcdef0",
		},
		{
			name:  "no download url present",
			input: `echo "hello world"`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			got := ""

			// act
			if match := cursorVersionRegex.FindStringSubmatch(tt.input); match != nil {
				got = match[1]
			}

			// assert
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
