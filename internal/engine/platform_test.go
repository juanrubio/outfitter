package engine

import "testing"

func TestPlatformNoticeFromOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		content string
		goarch  string
		want    string
	}{
		{
			name:    "ubuntu 26.04",
			content: "PRETTY_NAME=\"Ubuntu 26.04.1 LTS\"\nNAME=\"Ubuntu\"\nVERSION_ID=\"26.04\"\n",
			goarch:  "amd64",
			want:    "Ubuntu 26.04 · amd64",
		},
		{
			name:    "debian unquoted",
			content: "NAME=Debian GNU/Linux\nVERSION_ID=13\n",
			goarch:  "arm64",
			want:    "Debian GNU/Linux 13 · arm64",
		},
		{
			name:    "name only",
			content: "NAME=\"Ubuntu\"\n",
			goarch:  "amd64",
			want:    "Ubuntu · amd64",
		},
		{
			name:    "empty falls back to linux",
			content: "",
			goarch:  "amd64",
			want:    "Linux · amd64",
		},
		{
			name:    "garbage falls back to linux",
			content: "not an os-release file\n",
			goarch:  "riscv64",
			want:    "Linux · riscv64",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PlatformNoticeFromOSRelease([]byte(test.content), test.goarch); got != test.want {
				t.Errorf("PlatformNoticeFromOSRelease() = %q, want %q", got, test.want)
			}
		})
	}
}
