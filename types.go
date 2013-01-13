package spack

import (
	"sort"
	"fmt"
	"bytes"
	"bufio"
	"encoding/binary"
)

type version struct {
	Version uint16
	Spec *typeSpec
}

type VersionedType struct {
	Name string
	Tag uint16
	Versions []*version
}

type TypeSet struct {
	Types map[string]*VersionedType
	LastTag uint16
}

type TypeError struct {
	Message string
}

func (te *TypeError) Error() string {
	return te.Message
}


type TaggedKey struct {
	Tag uint16
	Key string
}

type EncodedObject struct {
	TaggedKey []byte
	Value []byte
}

//var fieldSpecType *fieldType = makeFieldType(fieldType{})
//var taggedKeyType *fieldType = makeFieldType(TaggedKey{})

// -------------------------------

func NewTypeSet() *TypeSet {
	return &TypeSet{
		Types: make(map[string]*VersionedType),
		LastTag: 0,
	}
}

func (ts *TypeSet) RegisterType(name string) *VersionedType {
	var tag = ts.LastTag + 1
	ts.LastTag = tag

	t, ok := ts.Types[name]
	if !ok {
		t = &VersionedType{
			Name: name,
			Tag: tag,
			Versions: make([]*version, 0, 1),
		}
		ts.Types[name] = t
	}

	return t
}

func (ts *TypeSet) LoadType(vt *VersionedType) error {
	if ts.HasTag(vt.Tag) {
		return &TypeError{ fmt.Sprintf("Tag already exists: %d", vt.Tag) }
	}

	_, ok := ts.Types[vt.Name]
	if ok {
		return &TypeError{ fmt.Sprintf("Name already exists: %s", vt.Name) }
	}

	ts.Types[vt.Name] = vt

	if vt.Tag > ts.LastTag {
		ts.LastTag = vt.Tag
	}

	return nil
}

func (ts *TypeSet) Type(name string) *VersionedType {
	t, ok := ts.Types[name]
	if !ok {
		panic(fmt.Sprintf("No such type: %s", name))
	}
	return t
}

func (ts *TypeSet) HasTag(tag uint16) bool {
	for _, vt := range ts.Types {
		if vt.Tag == tag {
			return true
		}
	}
	return false
}

// func (ts *TypeSet) DumpTypes() []*EncodedObject {
// 	var objs = make([]*EncodedObject, 0, len(ts.Types))
// 	for _, vt := range ts.Types {
// 		objs = append(objs, vt.EncodeTypeInfo())
// 	}
// 	return objs
// }

// -------------------------------

func (vt *VersionedType) AddVersion(vers uint16, exemplar interface{}) error {
	if vt.GetVersion(vers) != nil {
		return &TypeError{ fmt.Sprintf("Version already exists: %s::%d", vt.Name, vers) }
	}

	var ft = makeTypeSpec(exemplar)

	vt.Versions = append(vt.Versions, &version{ vers, ft })
	sort.Sort(vt)

	return nil
}

func (vt *VersionedType) GetVersion(v uint16) *version {
	var cmp = func(i int) bool {
		return vt.Versions[i].Version <= v
	}
	var idx = sort.Search(vt.Len(), cmp)
	if idx < vt.Len() && vt.Versions[idx].Version == v {
		return vt.Versions[idx]
	}
	return nil
}

func (vt *VersionedType) Encode(key string, obj interface{}) (o *EncodedObject, err error) {

	if len(vt.Versions) == 0 {
		return nil, &TypeError{ fmt.Sprintf("No versions registered for %s", vt.Name) }
	}

	var v = vt.Versions[0]

	var buf = new(bytes.Buffer)
	var writer = bufio.NewWriter(buf)
	err = safeEncodeField(obj, v.Spec, writer)
	if err != nil {
		return nil, err
	}
	writer.Flush()
	var value = buf.Bytes()

	var taggedKey = encodeKey(key, vt.Tag)

	return &EncodedObject{
		TaggedKey: taggedKey,
		Value: value,
	}, nil
}

// func (vt *VersionedType) EncodeTypeInfo() *EncodedObject {
// 	var buf = new(bytes.Buffer)
// 	var writer = bufio.NewWriter(buf)
// 	err := safeEncodeField(vt.Spec, fieldSpecType, writer)
// 	if err != nil {
// 		panic(fmt.Sprintf("Error encoding type: %v", err))
// 	}
// 	return obj
// }

func (vt *VersionedType) Len() int {
	return len(vt.Versions)
}

func (vt *VersionedType) Less(i int, j int) bool {
	return vt.Versions[i].Version > vt.Versions[j].Version
}

func (vt *VersionedType) Swap(i int, j int) {
	var tmp = vt.Versions[i]
	vt.Versions[i] = vt.Versions[j]
	vt.Versions[j] = tmp
}

// -------------------------------

func encodeKey(key string, tag uint16) []byte {
	var keyBytes = []byte(key)
	var buf = bytes.NewBuffer(make([]byte, 0, len(keyBytes) + 2))
	binary.Write(buf, binary.BigEndian, tag)
	buf.Write(keyBytes)
	return buf.Bytes()
}
