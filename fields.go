package spack

import (
	"bytes"
	"bufio"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
)

const IGNORED_FIELD reflect.Kind = 254
const STRUCT_REFERENCE reflect.Kind = 255

type fieldType struct {
	Kind uint8
	Elem []*fieldType
	Label string
	StructName string
}

type structMap map[string]*fieldType

type TypeSpec struct {
	Structs structMap
	Top *fieldType
}


func (ft *fieldType) String() string {
	var inner string
	if ft.Elem == nil {
		inner = "(nil)"
	} else {
		var bits = make([]string, 0)
		for i := 0; i < len(ft.Elem); i++ {
			bits = append(bits, ft.Elem[i].String())
		}
		inner = fmt.Sprintf("[ %s ]", strings.Join(bits, ", "))
	}
	return fmt.Sprintf("{ %v, %#v, %#v, %v }", ft.Kind, ft.Label, ft.StructName, inner)
}

func MakeTypeSpec(exemplar interface{}) *TypeSpec {
	var structs = make(structMap)
	var top = makeFieldType(reflect.TypeOf(exemplar), structs)
	return &TypeSpec{
		Structs: structs,
		Top: top,
	}
}

func makeFieldType(typ reflect.Type, structs structMap) *fieldType {

	switch typ.Kind() {
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Bool,
		reflect.String:
		return &fieldType{ uint8(typ.Kind()), nil, "", "" }

	case reflect.Slice:
		var elemType = makeFieldType(typ.Elem(), structs)
		return &fieldType{ uint8(reflect.Slice), []*fieldType{ elemType }, "", "" }

	case reflect.Ptr:
		return &fieldType{ uint8(reflect.Ptr), []*fieldType {
				makeFieldType(typ.Elem(), structs) }, "", "" }

	case reflect.Struct:

		var structName = typ.PkgPath() + "/" + typ.Name()
		structFt, ok := structs[structName]
		if !ok {
			structs[structName] = nil // Avoid reentrance
			var elems = make([]*fieldType, 0, typ.NumField())
			for i := 0; i < typ.NumField(); i++ {
				var field = typ.Field(i)

				var ft *fieldType

				if field.Tag.Get("spack") == "ignore" {
					ft = &fieldType{ uint8(IGNORED_FIELD), nil, field.Name, "" }
				} else {
					ft = makeFieldType(field.Type, structs)
					ft.Label = field.Name
				}

				elems = append(elems, ft)
			}
			structFt = &fieldType{ uint8(reflect.Struct), elems, "", "" }
			structs[structName] = structFt
		}
		
		return &fieldType{ uint8(STRUCT_REFERENCE), nil, "", structName }

	case reflect.Map:
		var keyType = makeFieldType(typ.Key(), structs)
		var valType = makeFieldType(typ.Elem(), structs)
		return &fieldType{ uint8(reflect.Map), []*fieldType{ keyType, valType }, "", "" }

	default:
	}

	panic(fmt.Sprintf("Can't make field type for %v\n", typ.Kind()))

}

func encodeField(field interface{}, ts *TypeSpec, writer *bufio.Writer) {
	encodeFieldInner(field, ts.Top, ts.Structs, writer)
}

func SafeEncodeField(field interface{}, ts *TypeSpec, writer *bufio.Writer) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = &TypeError{ 
				fmt.Sprintf("Encoding failed: %v", e),
			}
		}
	}()
	encodeFieldInner(field, ts.Top, ts.Structs, writer)
	return nil
}

func EncodeToBytes(field interface{}, ts *TypeSpec) ([]byte, error) {
	var buf bytes.Buffer
	var writer = bufio.NewWriter(&buf)
	var err = SafeEncodeField(field, ts, writer)
	if err != nil {
		return nil, err
	}
	writer.Flush()
	return buf.Bytes(), nil
}

func encodeFieldInner(field interface{}, ft *fieldType, structs structMap, writer *bufio.Writer) {

	switch reflect.Kind(ft.Kind) {
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128: 
		encodeFixedSize(field, ft.Kind, writer)

	case reflect.Bool:
		var n int
		var err error
		if field.(bool) {
			n, err = writer.Write([]byte{ 1 })
		} else {
			n, err = writer.Write([]byte{ 0 })
		}

		if n != 1 || err != nil {
			panic(fmt.Sprintf("Bool encode error: %v\n", err))
		}
		
	case reflect.String:
		var str = field.(string)
		writeLength(len(str), writer)
		_, err := writer.WriteString(str)
		if err != nil {
			panic(fmt.Sprintf("String length encode error: %v\n", err))
		}

	case reflect.Slice:
		var val = reflect.ValueOf(field)
		var sliceLen = val.Len()
		writeLength(sliceLen, writer)
		for i := 0; i < sliceLen; i++ {
			encodeFieldInner(val.Index(i).Interface(), ft.Elem[0], structs, writer)
		}

	case reflect.Map:
		var val = reflect.ValueOf(field)
		var keyCount = val.Len()
		writeLength(keyCount, writer)
		var keys = val.MapKeys()
		for _, key := range keys {
			encodeFieldInner(key.Interface(), ft.Elem[0], structs, writer)
			var value = val.MapIndex(key)
			encodeFieldInner(value.Interface(), ft.Elem[1], structs, writer)
		}

	case reflect.Ptr:

		var valType = reflect.TypeOf(field)
		var val = reflect.ValueOf(field)

		if valType == nil || val.IsNil() {
			writer.Write([]byte{ 0 })
		} else {
			writer.Write([]byte{ 1 })
			if valType.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			encodeFieldInner(val.Interface(), ft.Elem[0], structs, writer)
		}

	case IGNORED_FIELD:
		return

	case STRUCT_REFERENCE:
		var val = reflect.Indirect(reflect.ValueOf(field))

		var structFt = structs[ft.StructName]

		if val.Type().Kind() == reflect.Map {
			var mapVal = val.Interface().(map[string]interface{})
			for _, fieldFt := range structFt.Elem {
				if reflect.Kind(fieldFt.Kind) == IGNORED_FIELD {
					continue
				}
				var fieldVal = mapVal[fieldFt.Label]
				encodeFieldInner(fieldVal, fieldFt, structs, writer)
			}
		} else {

			var valName = val.Type().PkgPath() + "/" + val.Type().Name()
			if valName != ft.StructName {
				panic(fmt.Sprintf("Incompatible structs: %s, %s", valName, ft.StructName))
			}

			for i, fieldFt := range structFt.Elem {
				// Unexported fields aren't accessible, so we need to
				// check this here so they can at least be ignored
				if reflect.Kind(fieldFt.Kind) == IGNORED_FIELD {
					continue
				}
				encodeFieldInner(val.Field(i).Interface(), fieldFt, structs, writer)
			}
		}

	default:
		panic(fmt.Sprintf("Unsupported encode kind %v\n", ft.Kind))
	}
}

func writeLength(length int, writer *bufio.Writer) {
	var buf = make([]byte, binary.MaxVarintLen64)
	var lenLen = binary.PutUvarint(buf, uint64(length))
	_, err := writer.Write(buf[:lenLen])
	if err != nil {
		panic(fmt.Sprintf("String length encode error: %v\n", err))
	}
}


func decodeField(field interface{}, ts *TypeSpec, reader *bufio.Reader) {
	decodeFieldInner(field, ts.Top, ts.Structs, reader)
}

func SafeDecodeField(field interface{}, ts *TypeSpec, reader *bufio.Reader) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = &TypeError{ 
				fmt.Sprintf("Decoding failed: %v", e),
			}
		}
	}()
	decodeFieldInner(field, ts.Top, ts.Structs, reader)
	return nil
}

func DecodeFromBytes(field interface{}, ts *TypeSpec, enc []byte) (err error) {
	var buf = bytes.NewBuffer(enc)
	var reader = bufio.NewReader(buf)
	return SafeDecodeField(field, ts, reader)
}

func decodeFieldInner(field interface{}, ft *fieldType, structs structMap, reader *bufio.Reader) {

	switch reflect.Kind(ft.Kind) {
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128:
		var err = binary.Read(reader, binary.BigEndian, field)
		if err != nil {
			panic(fmt.Sprintf("Fixed size decode error: %v\n", err))
		}

	case reflect.Bool:
		var byte = make([]byte, 1)
		n, err := reader.Read(byte)
		if n != 1 || err != nil {
			panic(fmt.Sprintf("Bool decode error: %v\n", err))
		}

		if byte[0] == 0 {
			*field.(*bool) = false
		} else if byte[0] == 1 {
			*field.(*bool) = true
		} else {
			panic(fmt.Sprintf("Bool byte neither 0 nor 1: %v", byte[0]))
		}

	case reflect.String:
		byteLen, err := binary.ReadUvarint(reader)
		if err != nil {
			panic(fmt.Sprintf("Couldn't read string length: %v\n", err))
		}

		var runes = make([]rune, 0, byteLen)
		var byteCount uint64 = 0

		for byteCount < byteLen {
			rune, n, err := reader.ReadRune()
			if err != nil {
				panic(fmt.Sprintf("Rune read error: %v\n", err))
			}
			runes = append(runes, rune)
			byteCount += uint64(n)
		}

		*field.(*string) = string(runes)

	case reflect.Slice:

		elemCount64, err := binary.ReadUvarint(reader)
		if err != nil {
			panic(fmt.Sprintf("Couldn't read slice length: %v\n", err))
		}
		var elemCount = int(elemCount64)

		resultv := reflect.ValueOf(field)
		slicev := resultv.Elem()
		elemt := slicev.Type().Elem()

		for i := 0; i < elemCount; i++ {
			slicev = slicev.Slice(0, i)

			var elemp reflect.Value
			if elemt.Kind() == reflect.Interface {
				elemp = reflect.ValueOf(createMapValue(ft.Elem[0]))
			} else {
				elemp = reflect.New(elemt)
			}


			decodeFieldInner(elemp.Interface(), ft.Elem[0], structs, reader)
			slicev = reflect.Append(slicev, elemp.Elem())
		}

		resultv.Elem().Set(slicev.Slice(0, elemCount))

	case reflect.Map:

		keyCount64, err := binary.ReadUvarint(reader)
		if err != nil {
			panic(fmt.Sprintf("Couldn't read key count: %v\n", err))
		}

		var keyCount = int(keyCount64)
		var resultv = reflect.ValueOf(field).Elem()

		if resultv.IsNil() {
			resultv.Set(reflect.MakeMap(resultv.Type()))
		}

		var keyt = resultv.Type().Key()
		var valt = resultv.Type().Elem()

		for i := 0; i < keyCount; i++ {
			var keyp = reflect.New(keyt)
			decodeFieldInner(keyp.Interface(), ft.Elem[0], structs, reader)
			var valp = reflect.New(valt)
			decodeFieldInner(valp.Interface(), ft.Elem[1], structs, reader)
			resultv.SetMapIndex(keyp.Elem(), valp.Elem())
		}


	case reflect.Ptr:
		c, err := reader.ReadByte()
		if err != nil {
			panic(fmt.Sprintf("Couldn't read ptr nil byte: %v\n", err))
		}

		if c != 0 {
			var val = reflect.ValueOf(field)
			var target = reflect.Indirect(val)

			if target.IsNil() {
				target.Set(reflect.New(target.Type().Elem()))
			}

			decodeFieldInner(target.Interface(), ft.Elem[0], structs, reader)
		}

	case IGNORED_FIELD:
		return

	case STRUCT_REFERENCE:
		
		var val = reflect.ValueOf(field)
		val = reflect.Indirect(val)

		var structFt = structs[ft.StructName]

		if val.Type().Kind() == reflect.Map {
			for _, fieldFt := range structFt.Elem {
				if reflect.Kind(fieldFt.Kind) == IGNORED_FIELD {
					continue
				}
				var key = fieldFt.Label
				var fieldVal = createMapValue(fieldFt)
				decodeFieldInner(fieldVal, fieldFt, structs, reader)
				if fieldVal == nil {
					val.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(nil))
				} else {
					val.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(fieldVal).Elem())
				}
			}
		} else {

			var valName = val.Type().PkgPath() + "/" + val.Type().Name()
			if valName != ft.StructName {
				panic(fmt.Sprintf("Incompatible structs: %s, %s", valName, ft.StructName))
			}

			for i, fieldFt := range structFt.Elem {
				if reflect.Kind(fieldFt.Kind) == IGNORED_FIELD {
					continue
				}
				var fieldVal = val.Field(i).Addr()
				decodeFieldInner(fieldVal.Interface(), fieldFt, structs, reader)
			}
		}

	default:
		panic(fmt.Sprintf("Unsupported decode kind %v\n", ft.Kind))
	}
}


func encodeFixedSize(field interface{}, kind uint8, writer *bufio.Writer) {

	// Deal with vague types from JSON data
	switch field.(type) {
	case int:
		field = convertIntToFixedSize(field, reflect.Kind(kind))
	case float64:
		field = convertFloatToFixedSize(field, reflect.Kind(kind))
	}

	var err = binary.Write(writer, binary.BigEndian, field)
	if err != nil {
		panic(fmt.Sprintf("Fixed size encode error: %v\n", err))
	}
}

func convertIntToFixedSize(field interface{}, kind reflect.Kind) interface{} {
	var out interface{} = field

	switch kind {
	case reflect.Int8: out = int8(field.(int))
	case reflect.Int16: out = int16(field.(int))
	case reflect.Int32: out = int32(field.(int))
	case reflect.Int64: out = int64(field.(int))
	case reflect.Uint8: out = uint8(field.(int))
	case reflect.Uint16: out = uint16(field.(int))
	case reflect.Uint32: out = uint32(field.(int))
	case reflect.Uint64: out = uint64(field.(int))
	}
	return out
}

func convertFloatToFixedSize(field interface{}, kind reflect.Kind) interface{} {
	var out interface{} = field

	switch kind {
	case reflect.Int8: out = int8(int(field.(float64)))
	case reflect.Int16: out = int16(int(field.(float64)))
	case reflect.Int32: out = int32(int(field.(float64)))
	case reflect.Int64: out = int64(int(field.(float64)))
	case reflect.Uint8: out = uint8(int(field.(float64)))
	case reflect.Uint16: out = uint16(int(field.(float64)))
	case reflect.Uint32: out = uint32(int(field.(float64)))
	case reflect.Uint64: out = uint64(int(field.(float64)))
	}
	return out
}


func createMapValue(ft *fieldType) interface{} {
	switch reflect.Kind(ft.Kind) {
	case reflect.Int8:
		var val int8
		return &val

	case reflect.Int16:
		var val int16
		return &val

	case reflect.Int32:
		var val int32
		return &val

	case reflect.Int64:
		var val int64
		return &val

	case reflect.Uint8:
		var val uint8
		return &val

	case reflect.Uint16:
		var val uint16
		return &val

	case reflect.Uint32:
		var val uint32
		return &val
		
	case reflect.Uint64:
		var val uint64
		return &val

	case reflect.Float32:
		var val float32
		return &val

	case reflect.Float64:
		var val float64
		return &val

	case reflect.Complex64:
		var val complex64
		return &val

	case reflect.Complex128:
		var val complex128
		return &val

	case reflect.Bool:
		var val bool
		return &val

	case reflect.String:
		var val string
		return &val

	case reflect.Slice:
		var val = make([]interface{}, 0)
		return &val

	case reflect.Map:
		var val = make(map[interface{}]interface{})
		return &val

	case reflect.Ptr:
		var subVal = createMapValue(ft.Elem[0])
		return subVal

	case STRUCT_REFERENCE:
		var val = make(map[string]interface{})
		return &val
	}

	panic(fmt.Sprintf("Can't create map value for %v\n", ft))
}