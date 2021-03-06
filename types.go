package spack

import (
	"sort"
	"fmt"
	"bytes"
	"bufio"
	"encoding/binary"
	"reflect"
)

const BUFFER_SIZE = 256

type UpgradeFunc func(interface{}) (interface{}, error)

type Version struct {
	Version uint16
	Spec *TypeSpec
	Exemplar interface{} `spack:"ignore"`
	Upgrader UpgradeFunc `spack:"ignore"`
}

type VersionedType struct {
	Name string
	Tag uint16
	Versions []*Version
	Dirty bool `spack:"ignore"`
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


func EncodeKey(tag uint16, key string) []byte {
	var keyBytes = []byte(key)
	var buf = bytes.NewBuffer(make([]byte, 0, len(keyBytes) + 2))
	binary.Write(buf, binary.BigEndian, tag)
	buf.Write(keyBytes)
	return buf.Bytes()
}

// -------------------------------

func NewTypeSet() *TypeSet {
	var ts = &TypeSet{
		Types: make(map[string]*VersionedType),
		LastTag: 0,
	}

	var typeType = ts.RegisterType("_type")
	typeType.AddVersion(0, VersionedType{}, nil)

	return ts
}

func (ts *TypeSet) RegisterType(name string) *VersionedType {
	t, ok := ts.Types[name]
	if ok {
		return t
	}

	var tag = ts.LastTag + 1
	ts.LastTag = tag

	t = &VersionedType{
		Name: name,
		Tag: tag,
		Versions: make([]*Version, 0, 1),
		Dirty: true,
	}
	ts.Types[name] = t
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

// -------------------------------

func (vt *VersionedType) AddVersion(vers uint16, exemplar interface{}, upgrader UpgradeFunc) error {
	var _, v = vt.getVersion(vers)

	if v != nil {
		if v.Exemplar == nil && v.Upgrader == nil {
			v.Exemplar = exemplar
			v.Upgrader = upgrader
			return nil
		}
		return &TypeError{ fmt.Sprintf("Version already exists") }
	}

	var ft = MakeTypeSpec(exemplar)

	vt.AddVersionObj(&Version{ vers, ft, exemplar, upgrader })
	vt.Dirty = true

	return nil
}

func (vt *VersionedType) AddVersionObj(v *Version) {
	vt.Versions = append(vt.Versions, v)
	sort.Sort(vt)
}

func (vt *VersionedType) getVersion(v uint16) (int, *Version) {
	var cmp = func(i int) bool {
		return vt.Versions[i].Version <= v
	}
	var idx = sort.Search(vt.Len(), cmp)
	if idx < vt.Len() && vt.Versions[idx].Version == v {
		return idx, vt.Versions[idx]
	}
	return -1, nil
}

func (vt *VersionedType) GetVersion(version uint16) *Version {
	_, v := vt.getVersion(version)
	return v
}

func (vt *VersionedType) EncodeKey(key string) []byte {
	return EncodeKey(vt.Tag, key)
}

func (vt *VersionedType) EncodeTag() []byte {
	var buf = bytes.NewBuffer(make([]byte, 0, 2))
	binary.Write(buf, binary.BigEndian, vt.Tag)
	return buf.Bytes()
}

func (vt *VersionedType) DecodeKey(encKey []byte) string {
	return string(encKey[2:])
}

func (vt *VersionedType) EncodeObj(obj interface{}) (enc []byte, err error) {

	if len(vt.Versions) == 0 {
		return nil, &TypeError{ fmt.Sprintf("No versions registered for %s", vt.Name) }
	}

	var v = vt.Versions[0]

	var buf = bytes.NewBuffer(make([]byte, 0, BUFFER_SIZE))
	binary.Write(buf, binary.BigEndian, v.Version)

	var writer = bufio.NewWriter(buf)

	err = SafeEncodeField(obj, v.Spec, writer)
	if err != nil {
		return nil, err
	}
	writer.Flush()

	return buf.Bytes(), nil
}


func (vt *VersionedType) DecodeObj(encObj []byte, toMap bool) (obj interface{}, upgraded bool, err error) {

	if len(vt.Versions) == 0 {
		return nil, false, &TypeError{ fmt.Sprintf("No versions registered for %s", vt.Name) }
	}

	var buf = bytes.NewBuffer(encObj)

	var version uint16
	binary.Read(buf, binary.BigEndian, &version)

	var v = vt.Versions[0]

	if v.Version != version {
		return vt.upgradeObj(version, buf)
	}

	if v.Exemplar == nil {
		return nil, false, &TypeError{ fmt.Sprintf("Object version has no exemplar: %d", version) }
	}

	var target interface{}
	if toMap || v.Exemplar == nil {
		target = make(map[string]interface{})
	} else {
		target = reflect.New(reflect.TypeOf(v.Exemplar)).Interface()
	}

	var reader = bufio.NewReader(buf)
	err = SafeDecodeField(target, v.Spec, reader)

	if err != nil {
		return nil, false, err
	}

	return target, false, nil
}


func (vt *VersionedType) upgradeObj(version uint16, buf *bytes.Buffer) (obj interface{}, upgraded bool, err error) {
	var vIdx, v = vt.getVersion(version)

	if v == nil {
		return nil, false, &TypeError{ fmt.Sprintf("Version not registered: %d", version) }
	}

	if v.Exemplar != nil {
		obj = reflect.New(reflect.TypeOf(v.Exemplar)).Interface()
	} else {
		obj = make(map[string]interface{})
	}

	var reader = bufio.NewReader(buf)
	err = SafeDecodeField(obj, v.Spec, reader)

	if err != nil {
		return nil, false, &TypeError{ fmt.Sprintf("Error decoding initial version %d: %v", 
				v.Version, err) }
	}

	for vIdx > 0 {
		vIdx--
		var next = vt.Versions[vIdx]
		if next.Upgrader == nil {
			return nil, false, &TypeError{ fmt.Sprintf("No upgrader for %d -> %d (object version %d)", v.Version, next.Version, version) }
		}

		fmt.Printf("Upgrading %d -> %d\n", next.Version-1, next.Version)
		obj, err = next.Upgrader(obj)
		
		if err != nil {
			return nil, false, &TypeError{ fmt.Sprintf("Upgrader error: %v", err) }
		}
	}

	return obj, true, nil
}


func (vt *VersionedType) DecodeInto(encObj []byte, obj map[string]interface{}) error {
	if len(vt.Versions) == 0 {
		return &TypeError{ fmt.Sprintf("No versions registered for %s", vt.Name) }
	}

	var buf = bytes.NewBuffer(encObj)

	var version uint16
	binary.Read(buf, binary.BigEndian, &version)

	var _, v = vt.getVersion(version)

	if v == nil {
		return &TypeError{ fmt.Sprintf("Version not registered: %d", version) }
	}

	var reader = bufio.NewReader(buf)
	var err = SafeDecodeField(obj, v.Spec, reader)

	if err != nil {
		return err
	}

	obj["_version"] = v.Version

	return nil
}


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
