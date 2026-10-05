package formatter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/githoober/monogo"
	"github.com/githoober/monogo/formatter"
	"github.com/githoober/monogo/handler"
)

func TestLineFormatter(t *testing.T) {
	f := formatter.NewLine("", "")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := monogo.Record{
		Message: "test message",
		Level:   monogo.INFO,
		Channel: "main",
		Time:    now,
		Context: map[string]interface{}{"user_id": 42},
		Extra:   map[string]interface{}{"ip": "127.0.0.1"},
	}

	res, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	str := string(res)
	if !strings.Contains(str, "2025-01-01T12:00:00Z") {
		t.Errorf("expected timestamp in line format output, got: %s", str)
	}
	if !strings.Contains(str, "main.INFO: test message") {
		t.Errorf("expected channel/level/message, got: %s", str)
	}
	if !strings.Contains(str, `"user_id":42`) {
		t.Errorf("expected context JSON, got: %s", str)
	}
	if !strings.Contains(str, `"ip":"127.0.0.1"`) {
		t.Errorf("expected extra JSON, got: %s", str)
	}
}

func TestJSONFormatter(t *testing.T) {
	f := formatter.NewJSON("")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := monogo.Record{
		Message: "json test message",
		Level:   monogo.ERROR,
		Channel: "api",
		Time:    now,
		Context: map[string]interface{}{"error": "db timeout"},
	}

	res, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(res, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}

	if parsed["message"] != "json test message" {
		t.Errorf("unexpected message: %v", parsed["message"])
	}
	if parsed["level_name"] != "ERROR" {
		t.Errorf("unexpected level_name: %v", parsed["level_name"])
	}
	if parsed["channel"] != "api" {
		t.Errorf("unexpected channel: %v", parsed["channel"])
	}
	ctx, ok := parsed["context"].(map[string]interface{})
	if !ok || ctx["error"] != "db timeout" {
		t.Errorf("unexpected context: %v", parsed["context"])
	}
}

func TestLineFormatterBatch(t *testing.T) {
	f := formatter.NewLine("", "")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []monogo.Record{
		{Message: "msg 1", Level: monogo.INFO, Channel: "app", Time: now},
		{Message: "msg 2", Level: monogo.ERROR, Channel: "app", Time: now.Add(time.Second)},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(bytes))
	}
	if !strings.Contains(lines[0], "app.INFO: msg 1") {
		t.Errorf("expected line 1 to contain info msg, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "app.ERROR: msg 2") {
		t.Errorf("expected line 2 to contain error msg, got: %s", lines[1])
	}
}

func TestJSONFormatterBatchNewlines(t *testing.T) {
	f := formatter.NewJSON("")
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []monogo.Record{
		{Message: "batch 1", Level: monogo.INFO, Channel: "worker", Time: now},
		{Message: "batch 2", Level: monogo.WARNING, Channel: "worker", Time: now.Add(time.Second)},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var p1, p2 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &p1); err != nil {
		t.Fatalf("unmarshal line 1 error: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &p2); err != nil {
		t.Fatalf("unmarshal line 2 error: %v", err)
	}

	if p1["message"] != "batch 1" || p1["level_name"] != "INFO" {
		t.Errorf("unexpected payload 1: %v", p1)
	}
	if p2["message"] != "batch 2" || p2["level_name"] != "WARNING" {
		t.Errorf("unexpected payload 2: %v", p2)
	}
}

func TestJSONFormatterBatchJSON(t *testing.T) {
	f := formatter.NewJSON("").WithBatchMode(formatter.BatchModeJSON)
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []monogo.Record{
		{Message: "item 1", Level: monogo.DEBUG, Channel: "queue", Time: now},
		{Message: "item 2", Level: monogo.NOTICE, Channel: "queue", Time: now.Add(time.Second)},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("unmarshal JSON array error: %v (raw: %s)", err, string(bytes))
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 elements in JSON array, got %d", len(parsed))
	}
	if parsed[0]["message"] != "item 1" || parsed[0]["level_name"] != "DEBUG" {
		t.Errorf("unexpected first item: %v", parsed[0])
	}
	if parsed[1]["message"] != "item 2" || parsed[1]["level_name"] != "NOTICE" {
		t.Errorf("unexpected second item: %v", parsed[1])
	}
}

type customStringer struct {
	val string
}

func (c customStringer) String() string {
	return c.val
}

func TestLogfmtFormatter_Default(t *testing.T) {
	f := formatter.NewLogfmt()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := monogo.Record{
		Message: "User logged in",
		Level:   monogo.INFO,
		Channel: "auth",
		Time:    now,
		Context: map[string]interface{}{"user_id": 42, "role": "admin"},
		Extra:   map[string]interface{}{"ip": "127.0.0.1"},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	expected := "ts=2025-01-01T12:00:00Z lvl=INFO channel=auth msg=\"User logged in\" role=admin user_id=42 ip=127.0.0.1\n"
	if out != expected {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out, expected)
	}
}

func TestLogfmtFormatter_CustomKeys(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey("time"),
		formatter.WithLevelKey("level"),
		formatter.WithChannelKey("ch"),
		formatter.WithMessageKey("message"),
		formatter.WithLowercaseLevel(true),
	)
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := monogo.Record{
		Message: "payment succeeded",
		Level:   monogo.NOTICE,
		Channel: "billing",
		Time:    now,
		Context: map[string]interface{}{"amount": 99.5},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	expected := "time=2025-01-01T12:00:00Z level=notice ch=billing message=\"payment succeeded\" amount=99.5\n"
	if out != expected {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out, expected)
	}
}

func TestLogfmtFormatter_OmitKeysAndEmptyMessage(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithChannelKey(""),
	)

	rec := monogo.Record{
		Message: "",
		Level:   monogo.DEBUG,
		Context: map[string]interface{}{"k": "v"},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	expected := "lvl=DEBUG msg=\"\" k=v\n"
	if out != expected {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out, expected)
	}

	// Also test with empty messageKey
	fNoMsg := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
	)
	b2, err := fNoMsg.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}
	out2 := string(b2)
	expected2 := "lvl=DEBUG k=v\n"
	if out2 != expected2 {
		t.Errorf("unexpected output:\ngot:  %q\nwant: %q", out2, expected2)
	}
}

func TestLogfmtFormatter_QuotingAndEscaping(t *testing.T) {
	f := formatter.NewLogfmt(formatter.WithTimeKey(""), formatter.WithChannelKey(""))

	rec := monogo.Record{
		Message: "a \"quoted\" string with = and \n and \t",
		Level:   monogo.ERROR,
		Context: map[string]interface{}{
			"clean":     "hello",
			"empty":     "",
			"path":      "/api/v1/resource",
			"with_eq":   "foo=bar",
			"with_sp":   "hello world",
			"with_quot": `say "hi"`,
		},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, `msg="a \"quoted\" string with = and \n and \t"`) {
		t.Errorf("expected escaped message, got: %s", out)
	}
	if !strings.Contains(out, `clean=hello`) {
		t.Errorf("expected unquoted clean string, got: %s", out)
	}
	if !strings.Contains(out, `empty=""`) {
		t.Errorf("expected empty string quoted, got: %s", out)
	}
	if !strings.Contains(out, `path=/api/v1/resource`) {
		t.Errorf("expected unquoted path, got: %s", out)
	}
	if !strings.Contains(out, `with_eq="foo=bar"`) {
		t.Errorf("expected with_eq quoted, got: %s", out)
	}
	if !strings.Contains(out, `with_sp="hello world"`) {
		t.Errorf("expected with_sp quoted, got: %s", out)
	}
	if !strings.Contains(out, `with_quot="say \"hi\""`) {
		t.Errorf("expected with_quot quoted, got: %s", out)
	}
}

func TestLogfmtFormatter_ValueTypes(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
		formatter.WithLevelKey(""),
	)

	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	rec := monogo.Record{
		Context: map[string]interface{}{
			"t_nil":           nil,
			"t_bool_t":        true,
			"t_bool_f":        false,
			"t_int":           42,
			"t_int64":         int64(9223372036854775807),
			"t_int32":         int32(12345),
			"t_int16":         int16(123),
			"t_int8":          int8(12),
			"t_uint":          uint(99),
			"t_uint64":        uint64(18446744073709551615),
			"t_uint32":        uint32(54321),
			"t_uint16":        uint16(321),
			"t_uint8":         uint8(21),
			"t_float64":       3.14159,
			"t_float32":       float32(2.718),
			"t_time":          now,
			"t_duration":      250 * time.Millisecond,
			"t_err":           fmt.Errorf("connection timed out"),
			"t_stringer":      customStringer{val: "custom value"},
			"t_bytes":         []byte("byte string"),
			"t_slice":         []string{"x", "y"},
			"t_map":           map[string]int{"num": 1},
			"t_unmarshalable": make(chan int),
		},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	checks := []string{
		"t_nil=null",
		"t_bool_t=true",
		"t_bool_f=false",
		"t_int=42",
		"t_int64=9223372036854775807",
		"t_int32=12345",
		"t_int16=123",
		"t_int8=12",
		"t_uint=99",
		"t_uint64=18446744073709551615",
		"t_uint32=54321",
		"t_uint16=321",
		"t_uint8=21",
		"t_float64=3.14159",
		"t_time=2025-06-15T10:30:00Z",
		"t_duration=250ms",
		`t_err="connection timed out"`,
		`t_stringer="custom value"`,
		`t_bytes="byte string"`,
		`t_slice="[\"x\",\"y\"]"`,
		`t_map="{\"num\":1}"`,
		"t_unmarshalable=0x",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("expected output to contain %q, but got:\n%s", check, out)
		}
	}
}

func TestLogfmtFormatter_ContextAndExtraPrefixes(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithLevelKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
		formatter.WithContextPrefix("ctx."),
		formatter.WithExtraPrefix("extra."),
	)

	rec := monogo.Record{
		Context: map[string]interface{}{"user": "alice"},
		Extra:   map[string]interface{}{"ip": "1.2.3.4"},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(b)
	expected := "ctx.user=alice extra.ip=1.2.3.4\n"
	if out != expected {
		t.Errorf("unexpected output: got %q, want %q", out, expected)
	}
}

func TestLogfmtFormatter_CustomDateFormatWithSpaces(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithDateFormat("2006-01-02 15:04:05"),
		formatter.WithLevelKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
	)
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	b, err := f.Format(monogo.Record{Time: now})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(b)
	expected := "ts=\"2025-01-01 12:00:00\"\n"
	if out != expected {
		t.Errorf("unexpected output: got %q, want %q", out, expected)
	}
}

func TestLogfmtFormatter_KeySanitization(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithLevelKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
	)

	rec := monogo.Record{
		Context: map[string]interface{}{
			"user name": "alice",
			"a=b":       "c",
			`"quoted"`:  "val",
			"":          "empty key",
		},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, "user_name=alice") {
		t.Errorf("expected sanitized key user_name, got: %s", out)
	}
	if !strings.Contains(out, "a_b=c") {
		t.Errorf("expected sanitized key a_b, got: %s", out)
	}
	if !strings.Contains(out, "_quoted_=val") {
		t.Errorf("expected sanitized key _quoted_, got: %s", out)
	}
	if !strings.Contains(out, "_=\"empty key\"") {
		t.Errorf("expected empty key sanitized to _, got: %s", out)
	}
}

func TestLogfmtFormatter_FormatBatch(t *testing.T) {
	f := formatter.NewLogfmt(formatter.WithTimeKey(""), formatter.WithChannelKey(""))
	records := []monogo.Record{
		{Message: "msg 1", Level: monogo.INFO},
		{Message: "msg 2", Level: monogo.ERROR},
	}

	bytes, err := f.FormatBatch(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(bytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(bytes))
	}
	if lines[0] != `lvl=INFO msg="msg 1"` {
		t.Errorf("unexpected line 0: %q", lines[0])
	}
	if lines[1] != `lvl=ERROR msg="msg 2"` {
		t.Errorf("unexpected line 1: %q", lines[1])
	}
}

func assertLogfmtAlias(f *formatter.LogfmtFormatter) *formatter.LogfmtFormatter {
	return f
}

func TestLogfmtFormatter_AliasAndInterface(t *testing.T) {
	var _ monogo.Formatter = (*formatter.Logfmt)(nil)
	var _ monogo.BatchFormatter = (*formatter.Logfmt)(nil)

	f := formatter.NewLogfmt()
	fAlias := assertLogfmtAlias(f)
	if fAlias == nil {
		t.Fatal("expected non-nil formatter")
	}
}

func TestLogfmtFormatter_StreamHandlerIntegration(t *testing.T) {
	var buf bytes.Buffer
	f := formatter.NewLogfmt()
	sh := handler.NewStream(&buf, monogo.DEBUG, handler.WithFormatter(f))

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rec := monogo.Record{
		Message: "integrated stream test",
		Level:   monogo.INFO,
		Channel: "app",
		Time:    now,
		Context: map[string]interface{}{"status": "ok"},
	}

	if err := sh.Handle(context.Background(), rec); err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}

	expected := "ts=2025-01-01T12:00:00Z lvl=INFO channel=app msg=\"integrated stream test\" status=ok\n"
	if buf.String() != expected {
		t.Errorf("unexpected stream output:\ngot:  %q\nwant: %q", buf.String(), expected)
	}
}

type panickingNilError struct {
	msg string
}

func (e *panickingNilError) Error() string {
	return e.msg
}

type panickingNilStringer struct {
	val string
}

func (s *panickingNilStringer) String() string {
	return s.val
}

func TestLogfmtFormatter_TypedNilSafety(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithLevelKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
	)

	var nilErr *panickingNilError = nil
	var nilStr *panickingNilStringer = nil

	rec := monogo.Record{
		Context: map[string]interface{}{
			"err": nilErr,
			"str": nilStr,
		},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, "err=<nil>") {
		t.Errorf("expected err=<nil>, got: %s", out)
	}
	if !strings.Contains(out, "str=<nil>") {
		t.Errorf("expected str=<nil>, got: %s", out)
	}
}

func TestLogfmtFormatter_ControlCharsAndEscaping(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithLevelKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
	)

	rec := monogo.Record{
		Context: map[string]interface{}{
			"ctrl":  "null=\x00,bell=\a,vtab=\v",
			"html":  "a < b & c > d",
			"slash": `path\to\file`,
		},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, `ctrl="null=\u0000,bell=\u0007,vtab=\u000b"`) {
		t.Errorf("expected JSON-escaped control characters, got: %s", out)
	}
	if strings.Contains(out, `\x00`) || strings.Contains(out, `\a`) || strings.Contains(out, `\v`) {
		t.Errorf("found disallowed Go-style escapes in output: %s", out)
	}
	if !strings.Contains(out, `html="a < b & c > d"`) {
		t.Errorf("expected unescaped HTML characters, got: %s", out)
	}
	if !strings.Contains(out, `slash="path\\to\\file"`) {
		t.Errorf("expected escaped backslashes, got: %s", out)
	}
}

func TestLogfmtFormatter_UnicodeAndMalformedKeys(t *testing.T) {
	f := formatter.NewLogfmt(
		formatter.WithTimeKey(""),
		formatter.WithLevelKey(""),
		formatter.WithChannelKey(""),
		formatter.WithMessageKey(""),
	)

	malformedKey := string([]byte{0xff, 0xfe})
	nonPrintableKey := "user\u200Bname" // zero-width space
	printableUnicodeKey := "用户_ñ"

	rec := monogo.Record{
		Context: map[string]interface{}{
			malformedKey:        "invalid_utf8_key",
			nonPrintableKey:     "hidden_space",
			printableUnicodeKey: "valid_unicode",
		},
	}

	b, err := f.Format(rec)
	if err != nil {
		t.Fatalf("unexpected format error: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, "__=invalid_utf8_key") {
		t.Errorf("expected malformed key to be sanitized to __, got: %s", out)
	}
	if !strings.Contains(out, "user_name=hidden_space") {
		t.Errorf("expected zero-width space in key to be replaced with _, got: %s", out)
	}
	if !strings.Contains(out, "用户_ñ=valid_unicode") {
		t.Errorf("expected printable unicode key preserved, got: %s", out)
	}
}
