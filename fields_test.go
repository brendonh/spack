package spack

import (
	"testing"

	"bufio"
	"bytes"
	"reflect"
	_ "encoding/json"
)


func TestFieldType(test *testing.T) {

	type TypeTest struct {
		Exemplar interface{}
		Kind uint8
	}

	var tests = []TypeTest {
		TypeTest{ uint8(123), uint8(reflect.Uint8) },
		TypeTest{ complex64(123), uint8(reflect.Complex64) },
		TypeTest{ "Hello", uint8(reflect.String) },
	}

	for _, typeTest := range tests {
		var ft = MakeTypeSpec(typeTest.Exemplar)
		if ft.Top.Kind != typeTest.Kind {
			test.Errorf("Kind mismatch: %v vs %v", ft.Top.Kind, typeTest.Kind)
		}
	}

	var ft = MakeTypeSpec([]uint8{1, 2, 3})
	if ft.Top.Kind != uint8(reflect.Slice) {
		test.Errorf("Wrong kind for slice: %v\n", ft.Top.Kind)
	}
	if len(ft.Top.Elem) != 1 {
		test.Errorf("Wrong elem length for slice: %d\n", len(ft.Top.Elem))
	}

	if ft.Top.Elem[0].Kind != uint8(reflect.Uint8) {
		test.Errorf("Wrong elem for slice: %v\n", ft.Top.Elem[0])
	}
}

func TestStructSpec(test *testing.T) {
	type Struct struct {
		Name string
	}

	var ft = MakeTypeSpec(Struct{})
	
	if len(ft.Structs) != 1 {
		test.Error(ft)
	}

	if ft.Top.Kind != uint8(STRUCT_REFERENCE) {
		test.Error(ft)
	}
}

func TestDirectRecursionSpec(test *testing.T) {
	type Recursive struct {
		Inner *Recursive
	}

	var ft = MakeTypeSpec(Recursive{})

	if len(ft.Structs) != 1 {
		test.Error(ft)
	}

	if ft.Top.Kind != uint8(STRUCT_REFERENCE) {
		test.Error(ft)
	}
}

func TestDeepRecursiveSpec(test *testing.T) {
	type Recursive struct {
		Inner *Recursive
	}

	type Wrapper struct {
		Rec *Recursive
	}

	var ft = MakeTypeSpec(Wrapper{})

	if len(ft.Structs) != 2 {
		test.Error(ft)
	}
}



type _test_mutual_A struct {
	NameA string
	MB *_test_mutual_B
}

type _test_mutual_B struct {
	NameB string
	MA *_test_mutual_A
}

func TestMutualRecursiveSpec(test *testing.T) {
	var ft = MakeTypeSpec(_test_mutual_A{})

	if len(ft.Structs) != 2 {
		test.Error(ft)
	}

	ft = MakeTypeSpec(_test_mutual_B{})

	if len(ft.Structs) != 2 {
		test.Error(ft)
	}
}


func TestFixedSize(test *testing.T) {

	type FixedTest struct {
		Val interface{}
		Encoding []byte
	}

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
		var kind = kindSpec(typ.Kind())

		var buf bytes.Buffer
		var reader = bufio.NewReader(&buf)
		var writer = bufio.NewWriter(&buf)

		encodeField(t.Val, kind, writer)
		writer.Flush()

		var enc = buf.Bytes()

		if !reflect.DeepEqual(enc, t.Encoding) {
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
	var buf bytes.Buffer // = new(bytes.Buffer)
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(true, kindSpec(reflect.Bool), writer)
	writer.Flush()

	if !reflect.DeepEqual(buf.Bytes(), []byte { 1 }) {
		test.Errorf("Encoding wrong for true: %v", buf.Bytes())
	}

	var ret bool
	decodeField(&ret, kindSpec(reflect.Bool), reader)
	if !ret {
		test.Errorf("Decoding wrong for true: %v", ret)
	}

	buf.Reset()

	encodeField(false, kindSpec(reflect.Bool), writer)
	writer.Flush()

	if !reflect.DeepEqual(buf.Bytes(), []byte { 0 }) {
		test.Errorf("Encoding wrong for false: %v", buf.Bytes())
	}

	decodeField(&ret, kindSpec(reflect.Bool), reader)
	if ret {
		test.Errorf("Decoding wrong for false: %v", ret)
	}
}


func TestString(test *testing.T) {
	var tryString = func(str string) {

		var buf bytes.Buffer
		var reader = bufio.NewReader(&buf)
		var writer = bufio.NewWriter(&buf)
		
		encodeField(str, kindSpec(reflect.String), writer)
		writer.Flush()

		var dec string
		decodeField(&dec, kindSpec(reflect.String), reader)

		if dec != str {
			test.Errorf("String roundtrip failed: %v vs %v", str, dec)
		}
	}

	tryString("Hello World")
	tryString("世界您好")
	tryString("")
}

func TestByteSlice(test *testing.T) {
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)
	
	var orig = []uint8{ 1, 2, 34, 250 }

	var ft = MakeTypeSpec(orig)

	var dec = make([]uint8, 0)

	encodeField(orig, ft, writer)
	writer.Flush()
	
	decodeField(&dec, ft, reader)
	
	if !reflect.DeepEqual(orig, dec) {
		test.Errorf("Byte slice mismatch: %v vs %v", orig, dec)
	}
}


func TestStringSlice(test *testing.T) {
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)
	
	var orig = []string{ "one", "two", "thirty four" }
	var ft = MakeTypeSpec(orig)
	var dec []string

	encodeField(orig, ft, writer)
	writer.Flush()
	
	decodeField(&dec, ft, reader)
	
	if !reflect.DeepEqual(orig, dec) {
		test.Errorf("String slice mismatch: %v vs %v", orig, dec)
	}
}

func TestSliceSlice(test *testing.T) {
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)
	
	var orig = [][]uint8{ 
		[]uint8 { 1, 2, 3 },
		[]uint8 { 4, 5, 6, 8 },
		[]uint8 { 8, 9 },
	}

	var ft = MakeTypeSpec(orig)
	encodeField(orig, ft, writer)
	writer.Flush()

	var dec = make([][]uint8, 0)
	decodeField(&dec, ft, reader)

	if len(orig) != len(dec) {
		test.Errorf("Slice slice length mismatch: %d vs %d", len(orig), len(dec))
	}

	for i := range orig {
		if !reflect.DeepEqual(orig[i], dec[i]) {
			test.Errorf("Inner slice mismatch: %v vs %v", orig[i], dec[i])
		}
	}
}

func TestPointer(test *testing.T) {
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var orig *uint8 = new(uint8)
	*orig = 5

	var ft = MakeTypeSpec(orig)

	encodeField(orig, ft, writer)
	writer.Flush()

	if !reflect.DeepEqual(buf.Bytes(), []byte{ 1, 5 }) {
		test.Errorf("Error encoding pointer to uint8: %v", buf.Bytes())
	}

	var dec *uint8 = new(uint8)
	decodeField(&dec, ft, reader)

	if *dec != 5 {
		test.Errorf("Error decoding pointer to uint8: %v\n", dec)
	}
}


func TestSimpleStruct(test *testing.T) {

	type SimpleStruct struct {
		Name string
		Age uint32
	}

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var st = SimpleStruct{ "Brendon", 31 }
	var ft = MakeTypeSpec(st)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = SimpleStruct{}
	decodeField(&dec, ft, reader)

	if dec.Name != "Brendon" {
		test.Errorf("Wrong name: %v\n", dec.Name)
	}

	if dec.Age != 31 {
		test.Errorf("Wrong age: %v\n", dec.Age)
	}	
}

func TestEmbeddedStruct(test *testing.T) {
	type Embedded struct {
		Name string
		Age uint32
	}
	type Top struct {
		Embed Embedded
	}

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var st = Top { Embedded { "Brendon", 31 } }
	var ft = MakeTypeSpec(st)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Top{}
	decodeField(&dec, ft, reader)

	if dec.Embed.Name != "Brendon" {
		test.Errorf("Wrong name: %v\n", dec.Embed.Name)
	}

	if dec.Embed.Age != 31 {
		test.Errorf("Wrong age: %v\n", dec.Embed.Age)
	}	
}

func TestEmbeddedStructSlice(test *testing.T) {
	type Embedded struct {
		Name string
		Age uint32
	}
	type Top struct {
		Embeds []Embedded
	}

	var buf bytes.Buffer // = new(bytes.Buffer)
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var st = Top { 
		[]Embedded{ 
			Embedded { "Brendon", 31 }, 
			Embedded { "Ben", 26 },
			Embedded { "Nai", 32 },
		},
	}
	var ft = MakeTypeSpec(st)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Top{}
	decodeField(&dec, ft, reader)

	if len(dec.Embeds) != 3 {
		test.Errorf("Wrong count: %v\n", dec.Embeds)
	}

	var testEmbed = func(tag string, st *Embedded, name string, age uint32) {
		if st.Name != name { 
			test.Errorf("Wrong name for %s: %v\n", tag, st.Name)
		}

		if st.Age != age { 
			test.Errorf("Wrong age for %s: %v\n", tag, st.Age)
		}
	}

	testEmbed("0", &dec.Embeds[0], "Brendon", 31)
	testEmbed("1", &dec.Embeds[1], "Ben", 26)
	testEmbed("2", &dec.Embeds[2], "Nai", 32)

}

func TestReferencedStruct(test *testing.T) {
	type Embedded struct {
		Name string
		Age uint32
	}
	type Top struct {
		Embed *Embedded
	}

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var st = Top { &Embedded { "Brendon", 31 } }
	var ft = MakeTypeSpec(st)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Top{}
	decodeField(&dec, ft, reader)

	if dec.Embed.Name != "Brendon" {
		test.Errorf("Wrong name: %v\n", dec.Embed.Name)
	}

	if dec.Embed.Age != 31 {
		test.Errorf("Wrong age: %v\n", dec.Embed.Age)
	}	
}

func TestReferencedStructSlice(test *testing.T) {
	type Embedded struct {
		Name string
		Age uint32
	}
	type Top struct {
		Embeds []*Embedded
	}

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var st = Top { 
		[]*Embedded{ 
			&Embedded { "Brendon", 31 }, 
			&Embedded { "Ben", 26 },
			&Embedded { "Nai", 32 },
		},
	}
	var ft = MakeTypeSpec(st)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Top{}
	decodeField(&dec, ft, reader)

	if len(dec.Embeds) != 3 {
		test.Errorf("Wrong count: %v\n", dec.Embeds)
	}

	var testEmbed = func(tag string, st *Embedded, name string, age uint32) {
		if st.Name != name { 
			test.Errorf("Wrong name for %s: %v\n", tag, st.Name)
		}

		if st.Age != age { 
			test.Errorf("Wrong age for %s: %v\n", tag, st.Age)
		}
	}

	testEmbed("0", dec.Embeds[0], "Brendon", 31)
	testEmbed("1", dec.Embeds[1], "Ben", 26)
	testEmbed("2", dec.Embeds[2], "Nai", 32)
}

func TestNilPointer(test *testing.T) {
	type NotReallyHere struct{
		Name string
	}

	type Top struct {
		Missing *NotReallyHere
	}

	var st = Top{ nil }
	var ft = MakeTypeSpec(st)

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Top{}
	decodeField(&dec, ft, reader)

	if dec.Missing != nil {
		test.Errorf("Wrong decode for nil: %v\n", dec)
	}
}


func TestZeroSlice(test *testing.T) {
	type Top struct {
		Stuff []int32
	}

	var st = Top{ }
	var ft = MakeTypeSpec(st)

	var buf bytes.Buffer // = new(bytes.Buffer)
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Top{}
	decodeField(&dec, ft, reader)

	if dec.Stuff != nil {
		test.Errorf("Wrong decode for nil: %v\n", dec)
	}
}

func TestDirectRecursion(test *testing.T) {
	type Recursive struct {
		Name string
		Rec *Recursive
	}

	var st = Recursive {
		"One",
		&Recursive {
			"Two", 
			&Recursive {
				"Three",
				nil,
			},
		},
	}
	var ft = MakeTypeSpec(st)

	var buf bytes.Buffer // = new(bytes.Buffer)
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Recursive{}
	decodeField(&dec, ft, reader)

	if dec.Name != "One" || 
		dec.Rec.Name != "Two" ||
		dec.Rec.Rec.Name != "Three" ||
		dec.Rec.Rec.Rec != nil {
		test.Error(dec)
	}
}

func TestMutualRecursion(test *testing.T) {
	var st = _test_mutual_A {
		"A1",
		&_test_mutual_B {
			"B1", 
			&_test_mutual_A {
				"A2",
				&_test_mutual_B {
					"B2",
					nil,
				},
			},
		},
	}

	var ft = MakeTypeSpec(st)

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = _test_mutual_A{}
	decodeField(&dec, ft, reader)

	if dec.NameA != "A1" ||
		dec.MB.NameB != "B1" ||
		dec.MB.MA.NameA != "A2" ||
		dec.MB.MA.MB.NameB != "B2" ||
		dec.MB.MA.MB.MA != nil {
		test.Error(dec)
	}
}	

func TestMap(test *testing.T) {
	type Struct struct {
		Name string
		Age uint32
	}

	var orig = map[string]*Struct {
		"Brend": &Struct { "Brendon", 31 },
		"Nai": &Struct{ "Nai Yu", 32 },
	}

	var ft = MakeTypeSpec(orig)

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(orig, ft, writer)
	writer.Flush()

	var dec = make(map[string]*Struct)

	decodeField(&dec, ft, reader)

	if len(dec) != 2 {
		test.Errorf("Wrong key count for decoded map: %v\n", dec)
	}

	if dec["Brend"].Name != "Brendon" {
		test.Errorf("Wrong name for decoded Brend: %v\n", dec["Brend"])
	}

	if dec["Nai"].Name != "Nai Yu" {
		test.Errorf("Wrong name for decoded Nai: %v\n", dec["Brend"])
	}
}

func TestMapField(test *testing.T) {
	type Struct struct {
		Map map[string]string
	}

	var m = map[string]string {
		"One": "Two",
		"Three": "Four",
	}
	
	var st = Struct{ m }
	var ft = MakeTypeSpec(st)
	
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(st, ft, writer)
	writer.Flush()

	var dec = Struct{}
	decodeField(&dec, ft, reader)

	if len(dec.Map) != 2 ||
	dec.Map["One"] != "Two" ||
	dec.Map["Three"] != "Four" {
		test.Error(dec)
	}
}

func TestNilMap(test *testing.T) {
	type Struct struct {
		Missing map[string]string
	}

	var st = Struct{ }
	var ft = MakeTypeSpec(st)
	
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(st, ft, writer)
	writer.Flush()

	var dec = Struct{}

	decodeField(&dec, ft, reader)

	if len(dec.Missing) != 0 {
		test.Error(dec)
	}
}


func TestFieldTypeEncode(test *testing.T) {
	type Struct struct {
		Name string
		Age uint32 
		Self *Struct
		Mutual *_test_mutual_A
	}

	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	var ft = *MakeTypeSpec(Struct{})
	var ftft = MakeTypeSpec(ft)

	encodeField(ft, ftft, writer)
	writer.Flush()

	var dec = TypeSpec{}
	decodeField(&dec, ftft, reader)

	if !reflect.DeepEqual(ft, dec) {
		test.Errorf("Decoded non-matching field type: \n%v\n%v", ft, dec)
	}
}


func TestStructAsMap(test *testing.T) {
	var buf bytes.Buffer
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	type Struct struct {
		Name string
		Age uint32
	}

	var st = Struct{ "Brendon", 31 }
	var ft = MakeTypeSpec(st)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = make(map[string]interface{})
	decodeField(dec, ft, reader)

	if len(dec) != 2 {
		test.Errorf("Wrong field count in map-decoded struct: %v\n", dec)
	}

	if dec["Name"] != "Brendon" {
		test.Errorf("Wrong name in map-decoded struct: %v\n", dec["Name"])
	}

	if dec["Age"] != uint32(31) {
		test.Errorf("Wrong age in map-decoded struct: %v\n", dec["Age"])
	}
}


func TestMapAsStruct(test *testing.T) {

	type Struct struct {
		Name string
		Age uint32
	}

	var ft = MakeTypeSpec(Struct{})

	var st = map[string]interface{} {
		"Name": "Brendon",
		"Age": 31,
	}

	var buf bytes.Buffer // = new(bytes.Buffer)
	var reader = bufio.NewReader(&buf)
	var writer = bufio.NewWriter(&buf)

	encodeField(&st, ft, writer)
	writer.Flush()

	var dec = Struct{}
	decodeField(&dec, ft, reader)

	if dec.Name != "Brendon" {
		test.Errorf("Wrong name in struct from map: %v\n", dec.Name)
	}

	if dec.Age != 31 {
		test.Errorf("Wrong age in struct from map: %v\n", dec.Age)
	}
}


func kindType(kind reflect.Kind) *fieldType {
	return &fieldType{ uint8(kind), []*fieldType{}, "", "" }
}

func kindSpec(kind reflect.Kind) *TypeSpec {
	return &TypeSpec{ 
		Structs: nil,
		Top: kindType(kind),
	}
}
