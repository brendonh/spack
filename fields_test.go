package spack

import (
	"testing"

	"bufio"
	"bytes"
	"reflect"
)


type TypeTest struct {
	Exemplar interface{}
	Kind uint8
}

type SimpleStruct struct {
	Name string
	Count uint32
}

type NestedStruct struct {
	Name string
	Ages []uint8
	Embedded SimpleStruct
	Embeddeds []SimpleStruct
	Simple *SimpleStruct
	Simples []*SimpleStruct
}
	

func TestFieldType(test *testing.T) {
	var tests = []TypeTest {
		TypeTest{ uint8(123), uint8(reflect.Uint8) },
		TypeTest{ complex64(123), uint8(reflect.Complex64) },
		TypeTest{ "Hello", uint8(reflect.String) },
	}

	for _, typeTest := range tests {
		var ft = makeFieldType(typeTest.Exemplar)
		if ft.Kind != typeTest.Kind {
			test.Errorf("Kind mismatch: %v vs %v", ft.Kind, typeTest.Kind)
		}
	}

	var ft = makeFieldType([]uint8{1, 2, 3})
	if ft.Kind != uint8(reflect.Slice) {
		test.Errorf("Wrong kind for slice: %v\n", ft.Kind)
	}
	if len(ft.Elem) != 1 {
		test.Errorf("Wrong elem length for slice: %d\n", len(ft.Elem))
	}

	if ft.Elem[0].Kind != uint8(reflect.Uint8) {
		test.Errorf("Wrong elem for slice: %v\n", ft.Elem[0])
	}
	
	// Just make sure they don't error
	ft = makeFieldType(SimpleStruct{})
	ft = makeFieldType(NestedStruct{})
}


type FixedTest struct {
	Val interface{}
	Encoding []byte
}

func TestFixedSize(test *testing.T) {

	var tests = []FixedTest {
		FixedTest{ int8(123), []byte{ 123 } },
		FixedTest{ int8(-123), []byte{ 133 } },
		FixedTest{ int16(12312), []byte{ 48, 24 } },
		FixedTest{ int16(-12312), []byte{ 207, 232 } },
		FixedTest{ int32(123123123), []byte{ 7, 86, 181, 179 } },
		FixedTest{ int32(-123123123), []byte{ 248, 169, 74, 77 } },
		FixedTest{ int64(123123123123123123), []byte{ 1, 181, 107, 212, 1, 99, 243, 179 } },
		FixedTest{ int64(-123), []byte{ 255, 255, 255, 255, 255, 255, 255, 133 } },

		FixedTest{ uint8(123), []byte{ 123 } },		
		FixedTest{ int16(12312), []byte{ 48, 24 } },
		FixedTest{ int32(123123123), []byte{ 7, 86, 181, 179 } },
		FixedTest{ int64(123123123123123123), []byte{ 1, 181, 107, 212, 1, 99, 243, 179 } },

		// Not sure whether this is a valid thing to test
		FixedTest{ float32(123.123123), []byte{ 66, 246, 63, 10 } },
		FixedTest{ float64(123123123.123123), []byte{ 65, 157, 90, 214, 204, 126, 19, 245 } },

		FixedTest{ complex64(123 + 231i), []byte{ 66, 246, 0, 0, 67, 103, 0, 0 } },
		FixedTest{ complex128(123 + 231i), []byte{ 64, 94, 192, 0, 0, 0, 0, 0, 64, 108, 224, 0, 0, 0, 0, 0 } },
	}

	for _, t := range tests {
		var typ = reflect.TypeOf(t.Val)
		var kind = kindType(typ.Kind())

		var buf = new(bytes.Buffer)
		var reader = bufio.NewReader(buf)
		var writer = bufio.NewWriter(buf)

		encodeField(t.Val, kind, writer)
		writer.Flush()

		var enc = buf.Bytes()

		if !compareByteArrays(enc, t.Encoding) {
			test.Errorf("Encoding wrong for %v::%v: %v vs %v",
				kind, t.Val, enc, t.Encoding)
		}

		var field = reflect.New(typ).Interface()
		decodeField(field, kind, reader)
		var decoded = reflect.Indirect(reflect.ValueOf(field)).Interface()

		if decoded != t.Val {
			test.Errorf("Decoding wrong for %v::%v: %v", 
				kind, t.Val, decoded)
		}
	}
}

func TestBool(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	encodeField(true, kindType(reflect.Bool), writer)
	writer.Flush()

	if !compareByteArrays(buf.Bytes(), []byte { 1 }) {
		test.Errorf("Encoding wrong for true: %v", buf.Bytes())
	}

	var ret bool
	decodeField(&ret, kindType(reflect.Bool), reader)
	if !ret {
		test.Errorf("Decoding wrong for true: %v", ret)
	}

	buf.Reset()

	encodeField(false, kindType(reflect.Bool), writer)
	writer.Flush()

	if !compareByteArrays(buf.Bytes(), []byte { 0 }) {
		test.Errorf("Encoding wrong for false: %v", buf.Bytes())
	}

	decodeField(&ret, kindType(reflect.Bool), reader)
	if ret {
		test.Errorf("Decoding wrong for false: %v", ret)
	}
}

func TestString(test *testing.T) {
	var tryString = func(str string) {

		var buf = new(bytes.Buffer)
		var reader = bufio.NewReader(buf)
		var writer = bufio.NewWriter(buf)
		
		encodeField(str, kindType(reflect.String), writer)
		writer.Flush()

		var dec string
		decodeField(&dec, kindType(reflect.String), reader)

		if dec != str {
			test.Errorf("String roundtrip failed: %v vs %v", str, dec)
		}
	}

	tryString("Hello World")
	tryString("世界您好")
	tryString("")
}

func TestByteSlice(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)
	
	var ft = &fieldType{ 
		uint8(reflect.Slice), 
		[]*fieldType{ kindType(reflect.Uint8) },
	}

	var orig = []uint8{ 1, 2, 34, 250 }
	var dec = make([]uint8, 0)

	encodeField(orig, ft, writer)
	writer.Flush()
	
	decodeField(&dec, ft, reader)
	
	if !compareByteArrays(orig, dec) {
		test.Errorf("Byte slice mismatch: %v vs %v", orig, dec)
	}
}


func TestStringSlice(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)
	
	var ft = &fieldType{ uint8(reflect.Slice), []*fieldType{ kindType(reflect.String) } }
	var orig = []string{ "one", "two", "thirty four" }
	var dec = make([]string, 0)

	encodeField(orig, ft, writer)
	writer.Flush()
	
	decodeField(&dec, ft, reader)
	
	if !compareStringArrays(orig, dec) {
		test.Errorf("String slice mismatch: %v vs %v", orig, dec)
	}
}

func TestSliceSlice(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)
	
	var ft0 = &fieldType{ uint8(reflect.Slice), []*fieldType{ kindType(reflect.Uint8) } }
	var ft = &fieldType{ uint8(reflect.Slice), []*fieldType{ ft0 } }

	var orig = [][]uint8{ 
		[]uint8 { 1, 2, 3 },
		[]uint8 { 4, 5, 6, 8 },
		[]uint8 { 8, 9 },
	}

	encodeField(orig, ft, writer)
	writer.Flush()

	var dec = make([][]uint8, 0)
	decodeField(&dec, ft, reader)

	if len(orig) != len(dec) {
		test.Errorf("Slice slice length mismatch: %d vs %d", len(orig), len(dec))
	}

	for i := range orig {
		if !compareByteArrays(orig[i], dec[i]) {
			test.Errorf("Inner slice mismatch: %v vs %v", orig[i], dec[i])
		}
	}
}

func TestPointer(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var ft = &fieldType{ 
		uint8(reflect.Ptr), 
		[]*fieldType{ &fieldType{ uint8(reflect.Uint8), nil } },
	}

	var orig *uint8 = new(uint8)
	*orig = 5

	encodeField(&orig, ft, writer)
	writer.Flush()

	if !compareByteArrays(buf.Bytes(), []byte{ 1, 5 }) {
		test.Errorf("Error encoding pointer to uint8: %v", buf.Bytes())
	}

	var dec *uint8 = new(uint8)
	decodeField(&dec, ft, reader)

	if *dec != 5 {
		test.Errorf("Error decoding pointer to uint8: %v\n", dec)
	}
}


func TestStruct(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var st = SimpleStruct{ "Brendon", 31 }
	var ft = makeFieldType(st)
	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = SimpleStruct{}
	decodeField(&dec, ft, reader)

	if dec.Name != "Brendon" {
		test.Errorf("Wrong name: %v\n", dec.Name)
	}

	if dec.Count != 31 {
		test.Errorf("Wrong count: %v\n", dec.Count)
	}	
}


func TestNestedStruct(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var simples = []*SimpleStruct{ 
		&SimpleStruct{ "Brendon", 31 },
		&SimpleStruct{ "Ben", 26 },
		&SimpleStruct{ "Nai", 32 },
	}

	var ns = NestedStruct{ 
		Name: "Some Guys",
		Ages: []uint8{ 23, 91, 0 },
		Embedded: *simples[0],
		Embeddeds: []SimpleStruct { 
			*simples[0], 
			*simples[1],
			*simples[2],
		},
		Simple: simples[1],
		Simples: simples,
	}
	var ft = makeFieldType(ns)

	encodeField(&ns, ft, writer)
	writer.Flush()

	var dec = NestedStruct{}
	decodeField(&dec, ft, reader)

	if dec.Name != "Some Guys" {
		test.Errorf("Wrong name: %v\n", dec.Name)
	}

	if !compareByteArrays(dec.Ages, []uint8{ 23, 91, 0 }) {
		test.Errorf("Wrong ages: %v\n", dec.Ages)
	}	

	var testSimple = func(tag string, st *SimpleStruct, name string, count uint32) {
		if st.Name != name { 
			test.Errorf("Wrong name for %s: %v\n", tag, st.Name)
		}

		if st.Count != count { 
			test.Errorf("Wrong count for %s: %v\n", tag, st.Count)
		}
	}

	testSimple("embedded", &dec.Embedded, "Brendon", 31)

	if len(dec.Embeddeds) != 3 {
		test.Errorf("Wrong embeddeds length: %v\n", len(dec.Embeddeds))
	}

	testSimple("embeddeds[0]", &dec.Embeddeds[0], "Brendon", 31)
	testSimple("embeddeds[1]", &dec.Embeddeds[1], "Ben", 26)
	testSimple("embeddeds[2]", &dec.Embeddeds[2], "Nai", 32)

	testSimple("simple", dec.Simple, "Ben", 26)

	if len(dec.Simples) != 3 {
		test.Errorf("Wrong simples length: %v\n", len(dec.Simples))
	}

	testSimple("simples[0]", dec.Simples[0], "Brendon", 31)
	testSimple("simples[1]", dec.Simples[1], "Ben", 26)
	testSimple("simples[2]", dec.Simples[2], "Nai", 32)
}

func TestNilZero(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var ns = NestedStruct{ 
		Name: "",
		Ages: []uint8{},
		Embedded: SimpleStruct{},
		Embeddeds: []SimpleStruct{},
		Simple: nil,
		Simples: nil,
	}
	var ft = makeFieldType(ns)

	encodeField(&ns, ft, writer)
	writer.Flush()

	var dec = NestedStruct{}
	decodeField(&dec, ft, reader)

	if dec.Embedded.Name != "" {
		test.Errorf("Empty embedded name not empty: %v", dec.Embedded.Name)
	}

	if len(dec.Embeddeds) != 0 {
		test.Errorf("Empty embeddeds length not zero: %#v", dec.Embeddeds)
	}

	if dec.Simple != nil {
		test.Errorf("Empty simple not nil: %#v", dec.Simple)
	}

	if dec.Simples != nil {
		test.Errorf("Empty simples length not zero: %#v", dec.Simples)
	}
}


func TestFieldTypeEncodeSimple(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var ft = makeFieldType(SimpleStruct{})
	var ftft = makeFieldType(ft)

	encodeField(&ft, ftft, writer)
	writer.Flush()

	var dec = &fieldType{}
	decodeField(&dec, ftft, reader)

	if !compareFieldTypes(ft, dec) {
		test.Errorf("Decoded non-matching field type: %v, %v", ft, dec)
	}
}


func TestFieldTypeEncodeNested(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var ft = makeFieldType(NestedStruct{})
	var ftft = makeFieldType(ft)

	encodeField(&ft, ftft, writer)
	writer.Flush()

	var dec = &fieldType{}
	decodeField(&dec, ftft, reader)

	if !compareFieldTypes(ft, dec) {
		test.Errorf("Decoded non-matching field type: %v, %v", ft, dec)
	}
}


func TestFieldTypeFieldTypeEncode(test *testing.T) {
	var buf = new(bytes.Buffer)
	var reader = bufio.NewReader(buf)
	var writer = bufio.NewWriter(buf)

	var ft = makeFieldType(SimpleStruct{})
	var ftft = makeFieldType(ft)
	var ftftft = makeFieldType(ftft)

	encodeField(&ftft, ftftft, writer)
	writer.Flush()

	var dec = &fieldType{}
	decodeField(&dec, ftftft, reader)

	if !compareFieldTypes(ftft, dec) {
		test.Errorf("Decoded non-matching field type: %v, %v", ft, dec)
	}

	// Because why not!
	var ftftftft = makeFieldType(ftftft)
	if !compareFieldTypes(ftftft, ftftftft) {
		test.Errorf("Field type type type mismatch: %v", ftftft, ftftftft)
	}
}

func compareByteArrays(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func compareStringArrays(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func compareFieldTypes(a *fieldType, b *fieldType) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	
	if len(a.Elem) != len(b.Elem) {
		return false
	}

	for i := range a.Elem {
		if !compareFieldTypes(a.Elem[i], b.Elem[i]) {
			return false
		}
	}

	return true
}

func kindType(kind reflect.Kind) *fieldType {
	return &fieldType{ uint8(kind), nil }
}

