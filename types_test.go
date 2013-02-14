package spack

import (
	"testing"
)

func TestRegistration(test *testing.T) {

	type st0 struct {
	}

	type st1 struct {
		Name string
	}

	var ts = NewTypeSet()
	var vt = ts.RegisterType("test")

	var err = vt.AddVersion(0, st0{}, nil)
	if err != nil {
		test.Errorf("Registration failed for version 0: %s", err)
	}

	err = vt.AddVersion(1, st1{}, nil)
	if err != nil {
		test.Errorf("Registration failed for version 1: %s", err)
	}

	err = vt.AddVersion(0, st0{}, nil)
	if err == nil {
		test.Errorf("Re-registration succeeded for version 0")
	}	
}


func TestEncodeKey(test *testing.T) {
	type st0 struct {
		Name string
	}

	var ts = NewTypeSet()
	var vt = ts.RegisterType("test")
	vt.AddVersion(0, st0{}, nil)

	var enc = vt.EncodeKey("one")

	var dec = vt.DecodeKey(enc)

	if dec != "one" {
		test.Errorf("Decoded incorrect key: %v", dec)
	}
}

func TestEncodeObj(test *testing.T) {
	type st0 struct {
		Name string
		Ignored string `spack:"ignore"`
		Another string
	}

	var ts = NewTypeSet()
	var vt = ts.RegisterType("test")
	vt.AddVersion(0, st0{}, nil)

	var obj = &st0{ "Obj", "Nothing", "Something" }
	
	enc, err := vt.EncodeObj(obj)

	if err != nil {
		test.Errorf("Encoding error: %v", err)
	}

	decIF, _, err := vt.DecodeObj(enc, false)

	if err != nil {
		test.Errorf("Decoding error: %v\n", err)
	}

	var dec = decIF.(*st0)

	if dec.Name != "Obj" || dec.Ignored != "" || dec.Another != "Something" {
		test.Errorf("Decoding mismatch: %#v", dec)
	}

	type st1 struct {
		Name string
		Age uint16
	}

	var obj2 = &st1{ "Obj2", 2 }

	_, err = vt.EncodeObj(obj2)

	if err == nil {
		test.Errorf("No encoding err when expected")
	}

	vt.AddVersion(1, st1{}, nil)

	enc2, err := vt.EncodeObj(obj2)

	if err != nil {
		test.Errorf("Encoding error: %v", err)
	}

	decIF, _, err = vt.DecodeObj(enc2, false)

	if err != nil {
		test.Errorf("Decoding error: %v", err)
	}

	var dec2 = decIF.(*st1)

	if dec2.Name != "Obj2" || dec2.Age != 2 {
		test.Errorf("Decoding mismatch: %#v", dec2)
	}

	var target = make(map[string]interface{})
	err = vt.DecodeInto(enc, target)

	if err != nil || target["Name"] != "Obj" || target["Another"] != "Something" {
		test.Errorf("Map decoding error: %v", err)
	}

	_, _, err = vt.DecodeObj(enc, false)

	if err == nil {
		test.Errorf("Expected upgrade error")
	}

}


func TestTypeEncode(test *testing.T) {

	type st0 struct {
		Name string
	}

	var ts = NewTypeSet()
	var vt = ts.RegisterType("test")

	vt.AddVersion(0, st0{}, nil)

	var typeType = ts.Type("_type")

	_, err := typeType.EncodeObj(vt)

	if err != nil {
		test.Errorf("Error encoding type info: %v\n", err)
	}

}


func TestUpgrade (test *testing.T) {
	type st0 struct {
		Name string
	}

	type st1 struct {
		Name string
		Age uint16
	}

	var st0to1 = func(obj0 interface{}) (interface{}, error) {
		var obj = obj0.(map[string]interface{})
		obj["Age"] = 32
		return obj, nil
	}

	type st2 struct {
		Age uint16
		Moniker string
	}

	var st1to2 = func(obj1 interface{}) (interface{}, error) {
		var obj = obj1.(map[string]interface{})
		obj["Moniker"] = obj["Name"]
		delete(obj, "Name")
		return obj, nil
	}

	var ts = NewTypeSet()
	var vt = ts.RegisterType("test")
	vt.AddVersion(0, st0{}, nil)

	enc, err := vt.EncodeObj(&st0{ "Brend" })

	if err != nil {
		test.Errorf("Encoding error: %v", err)
		return
	}

	vt.AddVersion(1, st1{}, st0to1)
	vt.AddVersion(2, st2{}, st1to2)

	vt.GetVersion(0).Exemplar = nil
	vt.GetVersion(1).Exemplar = nil

	out, _, err := vt.DecodeObj(enc, false)

	if err != nil {
		test.Errorf("Error decoding: %v", err)
	}

	enc, err = vt.EncodeObj(out)

	if err != nil {
		test.Errorf("Error re-encoding: %v", err)
	}

	final, _, err := vt.DecodeObj(enc, false)

	if err != nil {
		test.Errorf("Error decoding upgraded: %v", err)
	}
	
	var finalObj = final.(*st2)

	if finalObj.Age != 32 || finalObj.Moniker != "Brend" {
		test.Error(finalObj)
	}

}

