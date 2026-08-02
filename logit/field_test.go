package logit

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFieldFuncs_valueEncoding(t *testing.T) {
	errSample := errors.New("boom")

	tests := []struct {
		name  string
		fn    FieldFunc
		wantK string
		wantV string
	}{
		{name: "StringField", fn: StringField("s", "a"), wantK: "s", wantV: "a"},
		{name: "IntField", fn: IntField("i", -7), wantK: "i", wantV: "-7"},
		{name: "Int64Field", fn: Int64Field("i64", 99), wantK: "i64", wantV: "99"},
		{name: "UintField", fn: UintField("u", 3), wantK: "u", wantV: "3"},
		{name: "Uint64Field", fn: Uint64Field("u64", 1<<32), wantK: "u64", wantV: "4294967296"},
		{name: "Float64Field", fn: Float64Field("f64", 1.5), wantK: "f64", wantV: "1.5"},
		{name: "Float32Field", fn: Float32Field("f32", 2.25), wantK: "f32", wantV: "2.25"},
		{name: "BoolField_true", fn: BoolField("b", true), wantK: "b", wantV: "true"},
		{name: "BoolField_false", fn: BoolField("b", false), wantK: "b", wantV: "false"},
		{name: "ErrorField", fn: ErrorField("e", errSample), wantK: "e", wantV: "boom"},
		{name: "ErrorField_nil", fn: ErrorField("e", nil), wantK: "e", wantV: "<nil>"},
		{name: "AnyField", fn: AnyField("a", struct{ X int }{3}), wantK: "a", wantV: "{3}"},
		{name: "TimeDurationField_zero", fn: TimeDurationField("d", 0), wantK: "d", wantV: "0ms"},
		{name: "TimeDurationField_ms", fn: TimeDurationField("d", 350*time.Millisecond), wantK: "d", wantV: "350ms"},
		{name: "TimeDurationField_sub_ms", fn: TimeDurationField("d", 500*time.Microsecond), wantK: "d", wantV: "0.5ms"},
		{name: "TimeDurationField_one_s", fn: TimeDurationField("d", time.Second), wantK: "d", wantV: "1s"},
		{name: "TimeDurationField_one_half_s", fn: TimeDurationField("d", 1500*time.Millisecond), wantK: "d", wantV: "1.5s"},
		{name: "TimeDurationField_one_m", fn: TimeDurationField("d", time.Minute), wantK: "d", wantV: "1m0s"},
		{name: "TimeDurationField_one_m_frac", fn: TimeDurationField("d", time.Minute+30*time.Second+500*time.Millisecond), wantK: "d", wantV: "1m30.5s"},
		{name: "TimeDurationField_one_h", fn: TimeDurationField("d", time.Hour), wantK: "d", wantV: "1h0m0s"},
		{name: "TimeDurationField_neg", fn: TimeDurationField("d", -750*time.Millisecond), wantK: "d", wantV: "-750ms"},
		{name: "TimeDurationField_neg", fn: TimeDurationField("d", 999999*time.Hour), wantK: "d", wantV: "999999h0m0s"},
		{name: "TimeDurationField_one_half_s", fn: TimeDurationField("d", 1500*time.Millisecond), wantK: "d", wantV: "1.5s"},
		// 泛型方法
		{name: "IntOf_int", fn: IntOf("i", -7), wantK: "i", wantV: "-7"},
		{name: "IntOf_int8", fn: IntOf("i8", int8(127)), wantK: "i8", wantV: "127"},
		{name: "IntOf_int16", fn: IntOf("i16", int16(-32768)), wantK: "i16", wantV: "-32768"},
		{name: "IntOf_int32", fn: IntOf("i32", int32(2147483647)), wantK: "i32", wantV: "2147483647"},
		{name: "IntOf_int64", fn: IntOf("i64", int64(99)), wantK: "i64", wantV: "99"},
		{name: "UintOf_uint", fn: UintOf("u", uint(3)), wantK: "u", wantV: "3"},
		{name: "UintOf_uint8", fn: UintOf("u8", uint8(255)), wantK: "u8", wantV: "255"},
		{name: "UintOf_uint16", fn: UintOf("u16", uint16(65535)), wantK: "u16", wantV: "65535"},
		{name: "UintOf_uint32", fn: UintOf("u32", uint32(1<<31)), wantK: "u32", wantV: "2147483648"},
		{name: "UintOf_uint64", fn: UintOf("u64", uint64(1<<32)), wantK: "u64", wantV: "4294967296"},
		{name: "FloatOf_float32", fn: FloatOf("f32", float32(2.25)), wantK: "f32", wantV: "2.25"},
		{name: "FloatOf_float64", fn: FloatOf("f64", float64(1.5)), wantK: "f64", wantV: "1.5"},
		{name: "FieldOf_custom", fn: FieldOf("hex", 255, func(v int) string { return strings.ToUpper("0xff") }), wantK: "hex", wantV: "0XFF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, v := tt.fn().Value()
			if k != tt.wantK || v != tt.wantV {
				t.Errorf("%s = (%q, %q) want (%q, %q)", tt.name, k, v, tt.wantK, tt.wantV)
			}
		})
	}
}

func TestFieldFuncs_stringAbs(t *testing.T) {
	tests := []struct {
		name   string
		fn     func() FieldFunc
		wantK  string
		wantV  string
		substr []string // 至少包含的子串
	}{
		{
			name: "StringAbsField_empty",
			fn: func() FieldFunc { return StringAbsField("e", "") },
			wantK: "e",
			wantV: "",
		},
		{
			name: "StringAbsField_defaultShort",
			fn: func() FieldFunc { return StringAbsField("s", "hello") },
			wantK: "s",
			wantV: "hello",
		},
		{
			name: "StringAbsField_truncatesASCII",
			fn: func() FieldFunc {
				return StringAbsField("ta", strings.Repeat("x", 20000))
			},
			wantK:  "ta",
			substr: []string{"<len=20000 md5=", " preview=", "...", "...", ">"},
		},
		{
			name: "StringAbsField_truncatesByRune",
			fn: func() FieldFunc {
				return StringAbsField("t", strings.Repeat("中", 20000))
			},
			wantK:  "t",
			substr: []string{"<len=20000 md5=", " preview=", "...", "...", ">"},
		},
		{
			name: "StringAbsField_mixedWidthUTF8",
			fn: func() FieldFunc {
				return StringAbsField("mw", "abc"+strings.Repeat("中", 20000)+"xyz")
			},
			wantK:  "mw",
			substr: []string{"<len=", " md5=", " preview=", "...", "...", ">"},
		},
		{
			name: "StringAbsField_customFormat",
			fn: func() FieldFunc {
				return StringAbsField("cf", "data",
					WithStringAbsFormatFunc(func(input string) string {
						return "<<" + input + ">>"
					}),
				)
			},
			wantK: "cf",
			wantV: "<<data>>",
		},
		{
			name: "StringAbsField_negativeSizesClamped",
			fn: func() FieldFunc {
				return StringAbsField("nc", strings.Repeat("x", 5000),
					WithStringAbsHead(-1),
					WithStringAbsMiddle(-1),
					WithStringAbsTail(-1),
				)
			},
			wantK:  "nc",
			substr: []string{"<len=5000 md5="},
		},
		{
			name: "StringAbsField_overlapAdjustsTailWindow",
			fn: func() FieldFunc {
				return StringAbsField("oa", strings.Repeat("x", 10),
					WithStringAbsHead(5),
					WithStringAbsMiddle(5),
					WithStringAbsTail(5),
				)
			},
			wantK: "oa",
			wantV: strings.Repeat("x", 10),
		},
		{
			name: "stringAbsField_Value",
			fn: func() FieldFunc {
				f := &stringAbsField{key: "k", val: "v"}
				return func() Field { return f }
			},
			wantK: "k",
			wantV: "v",
		},
		{
			name: "StringAbsField_multipleOptsOrder",
			fn: func() FieldFunc {
				return StringAbsField("mo", strings.Repeat("x", 20000),
					WithStringAbsHead(100),
					WithStringAbsMiddle(50),
					WithStringAbsTail(100),
					WithAllowNewline(true),
				)
			},
			wantK:  "mo",
			substr: []string{"<len=20000 md5="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fn()
			k, v := f().Value()
			if k != tt.wantK {
				t.Errorf("%s: key = %q want %q", tt.name, k, tt.wantK)
			}
			if tt.substr == nil && v != tt.wantV {
				t.Errorf("%s: value = %q want %q", tt.name, v, tt.wantV)
			}
			for _, s := range tt.substr {
				if !strings.Contains(v, s) {
					t.Errorf("%s: value = %q missing substring %q", tt.name, v, s)
				}
			}
		})
	}
}

func TestJSONAbsField_Value(t *testing.T) {
	tests := []struct {
		name  string
		fn    FieldFunc
		wantK string
		wantV string
	}{
		{
			name:  "empty_object",
			fn:    JSONAbsField("e", struct{}{}),
			wantK: "e",
			wantV: "{}",
		},
		{
			name:  "nil_value",
			fn:    JSONAbsField("n", nil),
			wantK: "n",
			wantV: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, v := tt.fn().Value()
			if k != tt.wantK || v != tt.wantV {
				t.Errorf("%s = (%q, %q) want (%q, %q)", tt.name, k, v, tt.wantK, tt.wantV)
			}
		})
	}
}
