package spack

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
)

type fieldType struct {
	Kind uint8
	Elem []*fieldType
	Label string
}

type typeSet []reflect.Type

const RECURSE = 255

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
	return fmt.Sprintf("{ %v, %#v, %v }", ft.Kind, ft.Label, inner)
}

func makeFieldType(exemplar interface{}) *fieldType {
	var topType = reflect.TypeOf(exemplar)
	var seenTypes = make(typeSet, 0)
	return makeFieldTypeByType(topType, topType, seenTypes)
}

func makeFieldTypeByType(typ reflect.Type, top reflect.Type, seen typeSet) *fieldType {

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
		return &fieldType{ uint8(typ.Kind()), nil, "" }

	case reflect.Slice:
		var elemType = makeFieldTypeByType(typ.Elem(), top, seen)
		return &fieldType{ uint8(reflect.Slice), []*fieldType{ elemType }, "" }

	case reflect.Ptr:

		for _, seenType := range seen {
			if typ == seenType {
				if typ == top {
					return &fieldType{ uint8(reflect.Ptr), 
						[]*fieldType{ &fieldType{ RECURSE, nil, "" } }, "" }
				}
				panic(fmt.Sprintf("Recursive type! %v, %v", typ, seenType))
			}
		}

		seen = append(seen, typ)

		return &fieldType{ uint8(reflect.Ptr), []*fieldType {
				makeFieldTypeByType(typ.Elem(), top, seen) }, "" }

	case reflect.Struct:
		var elems = make([]*fieldType, 0, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			var field = typ.Field(i)
			var ft = makeFieldTypeByType(field.Type, top, seen)
			ft.Label = field.Name
			elems = append(elems, ft)
		}
		return &fieldType{ uint8(reflect.Struct), elems, "" }

	case reflect.Map:
		var keyType = makeFieldTypeByType(typ.Key(), top, seen)
		var valType = makeFieldTypeByType(typ.Elem(), top, seen)
		return &fieldType{ uint8(reflect.Map), []*fieldType{ keyType, valType }, "" } 

	default:
	}

	panic(fmt.Sprintf("Can't make field type for %v\n", typ.Kind()))

}


func encodeField(field interface{}, ft *fieldType, writer *bufio.Writer) {
	encodeFieldInner(field, ft, ft, writer)
}


func encodeFieldInner(field interface{}, ft *fieldType, ftTop *fieldType, writer *bufio.Writer) {
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
			encodeFieldInner(val.Index(i).Interface(), ft.Elem[0], ftTop, writer)
		}

	case reflect.Map:
		var val = reflect.ValueOf(field)
		var keyCount = val.Len()
		writeLength(keyCount, writer)
		var keys = val.MapKeys()
		for _, key := range keys {
			encodeFieldInner(key.Interface(), ft.Elem[0], ftTop, writer)
			var value = val.MapIndex(key)
			encodeFieldInner(value.Interface(), ft.Elem[1], ftTop, writer)
		}

	case reflect.Ptr:
		if ft.Elem[0].Kind == RECURSE {
			encodeFieldInner(field, ftTop, ftTop, writer)
			return
		}

		var valType = reflect.TypeOf(field)
		var val = reflect.ValueOf(field)

		if valType == nil || val.IsNil() {
			writer.Write([]byte{ 0 })
		} else {
			writer.Write([]byte{ 1 })
			if valType.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			encodeFieldInner(val.Interface(), ft.Elem[0], ftTop, writer)
		}

	case reflect.Struct:
		var val = reflect.Indirect(reflect.ValueOf(field))

		if val.Type().Kind() == reflect.Map {
			var mapVal = val.Interface().(map[string]interface{})
			for _, fieldFt := range ft.Elem {
				var fieldVal = mapVal[fieldFt.Label]
				encodeFieldInner(fieldVal, fieldFt, ftTop, writer)
			}
		} else {
			for i := 0; i < val.NumField(); i++ {
				encodeFieldInner(val.Field(i).Interface(), ft.Elem[i], ftTop, writer)
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

func decodeField(field interface{}, ft *fieldType, reader *bufio.Reader) {
	decodeFieldInner(field, ft, ft, reader)
}

func decodeFieldInner(field interface{}, ft *fieldType, ftTop *fieldType, reader *bufio.Reader) {
	
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


			decodeFieldInner(elemp.Interface(), ft.Elem[0], ftTop, reader)
			slicev = reflect.Append(slicev, elemp.Elem())
		}

		resultv.Elem().Set(slicev.Slice(0, elemCount))

	case reflect.Map:
		keyCount64, err := binary.ReadUvarint(reader)
		if err != nil {
			panic(fmt.Sprintf("Couldn't read key count: %v\n", err))
		}

		var keyCount = int(keyCount64)
		var resultv = reflect.ValueOf(field)
		var keyt = resultv.Type().Key()
		var valt = resultv.Type().Elem()

		for i := 0; i < keyCount; i++ {
			var keyp = reflect.New(keyt)
			decodeFieldInner(keyp.Interface(), ft.Elem[0], ftTop, reader)
			var valp = reflect.New(valt)
			decodeFieldInner(valp.Interface(), ft.Elem[1], ftTop, reader)
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

			if ft.Elem[0].Kind == RECURSE {
				decodeFieldInner(target.Interface(), ftTop.Elem[0], ftTop, reader)
				return
			}

			decodeFieldInner(target.Interface(), ft.Elem[0], ftTop, reader)
		}

	case reflect.Struct:
		var val = reflect.ValueOf(field)
		val = reflect.Indirect(val)

		if val.Type().Kind() == reflect.Map {
			for _, fieldFt := range ft.Elem {
				var key = fieldFt.Label
				var fieldVal = createMapValue(fieldFt)
				decodeFieldInner(fieldVal, fieldFt, ftTop, reader)
				val.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(fieldVal).Elem())
			}
		} else {
			for i := 0; i < val.NumField(); i++ {
				var fieldVal = val.Field(i).Addr()
				decodeFieldInner(fieldVal.Interface(), ft.Elem[i], ftTop, reader)
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

	case reflect.Struct:
		var val = make(map[string]interface{})
		return &val
	}

	return nil

}