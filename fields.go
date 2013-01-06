package spack

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
)

type fieldType struct {
	Kind reflect.Kind
	Elem []*fieldType
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
	return fmt.Sprintf("{ %v, %v }", ft.Kind, inner)
}


func makeFieldType(exemplar interface{}) *fieldType {
	return makeFieldTypeByType(reflect.TypeOf(exemplar))
}

func makeFieldTypeByType(typ reflect.Type) *fieldType {
	switch typ.Kind() {
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Bool,
		reflect.String:
		return &fieldType{ typ.Kind(), nil }

	case reflect.Slice:
		var elemType = makeFieldTypeByType(typ.Elem())
		return &fieldType{ typ.Kind(), []*fieldType{ elemType } }

	case reflect.Ptr:
		return makeFieldTypeByType(typ.Elem())

	case reflect.Struct:
		var elems = make([]*fieldType, 0, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			var field = typ.Field(i)
			var ft = makeFieldTypeByType(field.Type)
			elems = append(elems, ft)
		}
		return &fieldType{ reflect.Struct, elems }
	}

	panic(fmt.Sprintf("Can't make field type for %v\n", typ.Kind()))
}


func encodeField(field interface{}, ft *fieldType, writer *bufio.Writer) {
	switch ft.Kind {
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128:
		var err = binary.Write(writer, binary.BigEndian, field)
		if err != nil {
			panic(fmt.Sprintf("Fixed size encode error: %v\n", err))
		}

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
			encodeField(val.Index(i).Interface(), ft.Elem[0], writer)
		}

	case reflect.Ptr:
		var val = reflect.ValueOf(field)
		encodeField(val.Elem().Interface(), ft.Elem[0], writer)

	case reflect.Struct:
		var val = reflect.Indirect(reflect.ValueOf(field))
		for i := 0; i < val.NumField(); i++ {
			encodeField(val.Field(i).Interface(), ft.Elem[i], writer)
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


func decodeField(field interface{}, ft *fieldType, reader *bufio.Reader) {
	
	switch ft.Kind {
	case reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
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
			elemp := reflect.New(elemt)
			decodeField(elemp.Interface(), ft.Elem[0], reader)
			slicev = reflect.Append(slicev, elemp.Elem())
		}

		resultv.Elem().Set(slicev.Slice(0, elemCount))

	case reflect.Ptr:
		var target = reflect.Indirect(reflect.ValueOf(field)).Interface()
		decodeField(target, ft.Elem[0], reader)

	case reflect.Struct:
		fmt.Printf("Decode struct: %#v (%s)\n", field, ft)
		var val = reflect.Indirect(reflect.ValueOf(field))
		for i := 0; i < val.NumField(); i++ {
			var fieldVal = val.Field(i).Addr()
			fmt.Printf("Field: %v (%s): %v\n", fieldVal, ft.Elem[i], fieldVal.IsNil())
			decodeField(fieldVal.Interface(), ft.Elem[i], reader)
		}

	default:
		panic(fmt.Sprintf("Unsupported decode kind %v\n", ft.Kind))
	}
}

