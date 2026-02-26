package config

import (
	"testing"
	"time"
)

// helper builds a lookup function from a map.
func envMap(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

// --- test structs ---

type flatConfig struct {
	Concurrency    int
	BufferSize     int
	ProcessTimeout time.Duration
	ShutdownTimeout time.Duration
}

type namedStrategy int

type configWithNamedType struct {
	Strategy namedStrategy
	Name     string
}

type innerPool struct {
	Workers    int
	BufferSize int
}

type nestedConfig struct {
	BufferSize      int
	RouterPool      innerPool
	ProcessTimeout  time.Duration
	ShutdownTimeout time.Duration
}

type embeddedBase struct {
	Concurrency int
	BufferSize  int
}

type configWithEmbed struct {
	embeddedBase
	MaxSize     int
	MaxDuration time.Duration
}

type allTypesConfig struct {
	S  string
	B  bool
	I  int
	I8 int8
	I16 int16
	I32 int32
	I64 int64
	U   uint
	U8  uint8
	U16 uint16
	U32 uint32
	U64 uint64
	F32 float32
	F64 float64
	D   time.Duration
}

type configWithFunc struct {
	Concurrency  int
	ErrorHandler func(error)
	BufferSize   int
}

type configWithInterface struct {
	Name   string
	Logger interface{ Log(string) }
	Size   int
}

type deepNested struct {
	Inner struct {
		Value int
	}
}

// --- Tests ---

func TestLoad_FlatConfig(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_TRANSFORM_CONCURRENCY":      "8",
			"GOPIPE_TRANSFORM_BUFFER_SIZE":       "256",
			"GOPIPE_TRANSFORM_PROCESS_TIMEOUT":   "5s",
			"GOPIPE_TRANSFORM_SHUTDOWN_TIMEOUT":  "30s",
		}),
	}

	var cfg flatConfig
	if err := l.Load("transform", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Concurrency != 8 {
		t.Errorf("Concurrency = %d, want 8", cfg.Concurrency)
	}
	if cfg.BufferSize != 256 {
		t.Errorf("BufferSize = %d, want 256", cfg.BufferSize)
	}
	if cfg.ProcessTimeout != 5*time.Second {
		t.Errorf("ProcessTimeout = %v, want 5s", cfg.ProcessTimeout)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
}

func TestLoad_NamedNestedStruct(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_ENGINE_BUFFER_SIZE":              "100",
			"GOPIPE_ENGINE_ROUTER_POOL_WORKERS":      "4",
			"GOPIPE_ENGINE_ROUTER_POOL_BUFFER_SIZE":  "200",
			"GOPIPE_ENGINE_PROCESS_TIMEOUT":          "30s",
			"GOPIPE_ENGINE_SHUTDOWN_TIMEOUT":         "5s",
		}),
	}

	var cfg nestedConfig
	if err := l.Load("engine", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.BufferSize != 100 {
		t.Errorf("BufferSize = %d, want 100", cfg.BufferSize)
	}
	if cfg.RouterPool.Workers != 4 {
		t.Errorf("RouterPool.Workers = %d, want 4", cfg.RouterPool.Workers)
	}
	if cfg.RouterPool.BufferSize != 200 {
		t.Errorf("RouterPool.BufferSize = %d, want 200", cfg.RouterPool.BufferSize)
	}
	if cfg.ProcessTimeout != 30*time.Second {
		t.Errorf("ProcessTimeout = %v, want 30s", cfg.ProcessTimeout)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 5s", cfg.ShutdownTimeout)
	}
}

func TestLoad_EmbeddedStruct(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			// Embedded fields are flattened — no "EMBEDDED_BASE" segment.
			"GOPIPE_BATCH_CONCURRENCY":  "3",
			"GOPIPE_BATCH_BUFFER_SIZE":  "50",
			"GOPIPE_BATCH_MAX_SIZE":     "100",
			"GOPIPE_BATCH_MAX_DURATION": "2s",
		}),
	}

	var cfg configWithEmbed
	if err := l.Load("batch", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want 3", cfg.Concurrency)
	}
	if cfg.BufferSize != 50 {
		t.Errorf("BufferSize = %d, want 50", cfg.BufferSize)
	}
	if cfg.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", cfg.MaxSize)
	}
	if cfg.MaxDuration != 2*time.Second {
		t.Errorf("MaxDuration = %v, want 2s", cfg.MaxDuration)
	}
}

func TestLoad_AllTypes(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_TYPES_S":   "hello",
			"GOPIPE_TYPES_B":   "true",
			"GOPIPE_TYPES_I":   "-42",
			"GOPIPE_TYPES_I8":  "-8",
			"GOPIPE_TYPES_I16": "-16",
			"GOPIPE_TYPES_I32": "-32",
			"GOPIPE_TYPES_I64": "-64",
			"GOPIPE_TYPES_U":   "42",
			"GOPIPE_TYPES_U8":  "8",
			"GOPIPE_TYPES_U16": "16",
			"GOPIPE_TYPES_U32": "32",
			"GOPIPE_TYPES_U64": "64",
			"GOPIPE_TYPES_F32": "3.14",
			"GOPIPE_TYPES_F64": "2.718",
			"GOPIPE_TYPES_D":   "500ms",
		}),
	}

	var cfg allTypesConfig
	if err := l.Load("types", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.S != "hello" {
		t.Errorf("S = %q, want %q", cfg.S, "hello")
	}
	if cfg.B != true {
		t.Errorf("B = %v, want true", cfg.B)
	}
	if cfg.I != -42 {
		t.Errorf("I = %d, want -42", cfg.I)
	}
	if cfg.I8 != -8 {
		t.Errorf("I8 = %d, want -8", cfg.I8)
	}
	if cfg.I16 != -16 {
		t.Errorf("I16 = %d, want -16", cfg.I16)
	}
	if cfg.I32 != -32 {
		t.Errorf("I32 = %d, want -32", cfg.I32)
	}
	if cfg.I64 != -64 {
		t.Errorf("I64 = %d, want -64", cfg.I64)
	}
	if cfg.U != 42 {
		t.Errorf("U = %d, want 42", cfg.U)
	}
	if cfg.U8 != 8 {
		t.Errorf("U8 = %d, want 8", cfg.U8)
	}
	if cfg.U16 != 16 {
		t.Errorf("U16 = %d, want 16", cfg.U16)
	}
	if cfg.U32 != 32 {
		t.Errorf("U32 = %d, want 32", cfg.U32)
	}
	if cfg.U64 != 64 {
		t.Errorf("U64 = %d, want 64", cfg.U64)
	}
	if cfg.F32 != 3.14 {
		t.Errorf("F32 = %f, want 3.14", cfg.F32)
	}
	if cfg.F64 != 2.718 {
		t.Errorf("F64 = %f, want 2.718", cfg.F64)
	}
	if cfg.D != 500*time.Millisecond {
		t.Errorf("D = %v, want 500ms", cfg.D)
	}
}

func TestLoad_NamedType(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_HANDLER_STRATEGY": "2",
			"GOPIPE_HANDLER_NAME":     "test",
		}),
	}

	var cfg configWithNamedType
	if err := l.Load("handler", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Strategy != 2 {
		t.Errorf("Strategy = %d, want 2", cfg.Strategy)
	}
	if cfg.Name != "test" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test")
	}
}

func TestLoad_CustomPrefix(t *testing.T) {
	l := Loader{
		Prefix: "MYAPP",
		lookup: envMap(map[string]string{
			"MYAPP_STAGE_CONCURRENCY": "12",
		}),
	}

	var cfg flatConfig
	if err := l.Load("stage", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Concurrency != 12 {
		t.Errorf("Concurrency = %d, want 12", cfg.Concurrency)
	}
}

func TestLoad_StageNormalization(t *testing.T) {
	tests := []struct {
		stage string
		key   string
	}{
		{"process-order", "GOPIPE_PROCESS_ORDER_CONCURRENCY"},
		{"My Stage", "GOPIPE_MY_STAGE_CONCURRENCY"},
		{"UPPER", "GOPIPE_UPPER_CONCURRENCY"},
		{"with_underscore", "GOPIPE_WITH_UNDERSCORE_CONCURRENCY"},
		{"mixed-Case_Name", "GOPIPE_MIXED_CASE_NAME_CONCURRENCY"},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			l := Loader{
				lookup: envMap(map[string]string{
					tt.key: "7",
				}),
			}

			var cfg flatConfig
			if err := l.Load(tt.stage, &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Concurrency != 7 {
				t.Errorf("Concurrency = %d, want 7 (key: %s)", cfg.Concurrency, tt.key)
			}
		})
	}
}

func TestLoad_MissingEnvVarsPreserveDefaults(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			// Only set Concurrency, leave BufferSize unset.
			"GOPIPE_STAGE_CONCURRENCY": "5",
		}),
	}

	cfg := flatConfig{BufferSize: 42}
	if err := l.Load("stage", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Concurrency != 5 {
		t.Errorf("Concurrency = %d, want 5", cfg.Concurrency)
	}
	if cfg.BufferSize != 42 {
		t.Errorf("BufferSize = %d, want 42 (preserved default)", cfg.BufferSize)
	}
}

func TestLoad_SkipsFuncAndInterfaceFields(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_CONCURRENCY": "3",
			"GOPIPE_STAGE_BUFFER_SIZE": "10",
		}),
	}

	var cfg configWithFunc
	if err := l.Load("stage", &cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want 3", cfg.Concurrency)
	}
	if cfg.BufferSize != 10 {
		t.Errorf("BufferSize = %d, want 10", cfg.BufferSize)
	}
	if cfg.ErrorHandler != nil {
		t.Error("ErrorHandler should remain nil")
	}
}

func TestLoad_InvalidInt(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_CONCURRENCY": "not_a_number",
		}),
	}

	var cfg flatConfig
	err := l.Load("stage", &cfg)
	if err == nil {
		t.Fatal("expected error for invalid int")
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_PROCESS_TIMEOUT": "bad",
		}),
	}

	var cfg flatConfig
	err := l.Load("stage", &cfg)
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestLoad_InvalidBool(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_B": "not_bool",
		}),
	}

	var cfg allTypesConfig
	err := l.Load("stage", &cfg)
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestLoad_InvalidFloat(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_F64": "not_float",
		}),
	}

	var cfg allTypesConfig
	err := l.Load("stage", &cfg)
	if err == nil {
		t.Fatal("expected error for invalid float")
	}
}

func TestLoad_InvalidUint(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_U": "-1",
		}),
	}

	var cfg allTypesConfig
	err := l.Load("stage", &cfg)
	if err == nil {
		t.Fatal("expected error for invalid uint")
	}
}

func TestLoad_NotAPointer(t *testing.T) {
	l := Loader{lookup: envMap(nil)}
	err := l.Load("stage", flatConfig{})
	if err == nil {
		t.Fatal("expected error for non-pointer dst")
	}
}

func TestLoad_NotAStruct(t *testing.T) {
	l := Loader{lookup: envMap(nil)}
	n := 42
	err := l.Load("stage", &n)
	if err == nil {
		t.Fatal("expected error for non-struct dst")
	}
}

func TestLoad_DeepNested(t *testing.T) {
	l := Loader{
		lookup: envMap(map[string]string{
			"GOPIPE_STAGE_INNER_VALUE": "99",
		}),
	}

	var cfg deepNested
	if err := l.Load("stage", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Inner.Value != 99 {
		t.Errorf("Inner.Value = %d, want 99", cfg.Inner.Value)
	}
}

func TestKeys_FlatConfig(t *testing.T) {
	keys := Keys("transform", flatConfig{})
	want := []string{
		"GOPIPE_TRANSFORM_CONCURRENCY",
		"GOPIPE_TRANSFORM_BUFFER_SIZE",
		"GOPIPE_TRANSFORM_PROCESS_TIMEOUT",
		"GOPIPE_TRANSFORM_SHUTDOWN_TIMEOUT",
	}
	assertKeys(t, keys, want)
}

func TestKeys_NestedConfig(t *testing.T) {
	keys := Keys("engine", nestedConfig{})
	want := []string{
		"GOPIPE_ENGINE_BUFFER_SIZE",
		"GOPIPE_ENGINE_ROUTER_POOL_WORKERS",
		"GOPIPE_ENGINE_ROUTER_POOL_BUFFER_SIZE",
		"GOPIPE_ENGINE_PROCESS_TIMEOUT",
		"GOPIPE_ENGINE_SHUTDOWN_TIMEOUT",
	}
	assertKeys(t, keys, want)
}

func TestKeys_EmbeddedConfig(t *testing.T) {
	keys := Keys("batch", configWithEmbed{})
	want := []string{
		"GOPIPE_BATCH_CONCURRENCY",
		"GOPIPE_BATCH_BUFFER_SIZE",
		"GOPIPE_BATCH_MAX_SIZE",
		"GOPIPE_BATCH_MAX_DURATION",
	}
	assertKeys(t, keys, want)
}

func TestKeys_SkipsFuncFields(t *testing.T) {
	keys := Keys("stage", configWithFunc{})
	want := []string{
		"GOPIPE_STAGE_CONCURRENCY",
		"GOPIPE_STAGE_BUFFER_SIZE",
	}
	assertKeys(t, keys, want)
}

func TestKeys_SkipsInterfaceFields(t *testing.T) {
	keys := Keys("stage", configWithInterface{})
	want := []string{
		"GOPIPE_STAGE_NAME",
		"GOPIPE_STAGE_SIZE",
	}
	assertKeys(t, keys, want)
}

func TestKeys_CustomPrefix(t *testing.T) {
	l := Loader{Prefix: "APP"}
	keys := l.Keys("worker", flatConfig{})
	want := []string{
		"APP_WORKER_CONCURRENCY",
		"APP_WORKER_BUFFER_SIZE",
		"APP_WORKER_PROCESS_TIMEOUT",
		"APP_WORKER_SHUTDOWN_TIMEOUT",
	}
	assertKeys(t, keys, want)
}

func TestKeys_Pointer(t *testing.T) {
	keys := Keys("stage", &flatConfig{})
	if len(keys) != 4 {
		t.Errorf("Keys with pointer: got %d keys, want 4", len(keys))
	}
}

func TestKeys_NonStruct(t *testing.T) {
	keys := Keys("stage", 42)
	if keys != nil {
		t.Errorf("Keys for non-struct: got %v, want nil", keys)
	}
}

func TestLoad_PackageLevelFunc(t *testing.T) {
	// This test uses real env vars.
	t.Setenv("GOPIPE_PKG_CONCURRENCY", "99")

	var cfg flatConfig
	if err := Load("pkg", &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Concurrency != 99 {
		t.Errorf("Concurrency = %d, want 99", cfg.Concurrency)
	}
}

func TestToUpperSnake(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"BufferSize", "BUFFER_SIZE"},
		{"ProcessTimeout", "PROCESS_TIMEOUT"},
		{"Concurrency", "CONCURRENCY"},
		{"MaxSize", "MAX_SIZE"},
		{"RouterPool", "ROUTER_POOL"},
		{"AckStrategy", "ACK_STRATEGY"},
		{"URLPath", "URL_PATH"},
		{"HTTPClient", "HTTP_CLIENT"},
		{"ID", "ID"},
		{"Workers", "WORKERS"},
		{"I", "I"},
		{"I8", "I8"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := toUpperSnake(tt.in)
			if got != tt.want {
				t.Errorf("toUpperSnake(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeStage(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"transform", "TRANSFORM"},
		{"process-order", "PROCESS_ORDER"},
		{"My Stage", "MY_STAGE"},
		{"UPPER", "UPPER"},
		{"with_underscore", "WITH_UNDERSCORE"},
		{"special!@#chars", "SPECIALCHARS"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := normalizeStage(tt.in)
			if got != tt.want {
				t.Errorf("normalizeStage(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %d keys, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("key[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
