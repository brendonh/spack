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

	var err = vt.AddVersion(0, st0{})
	if err != nil {
		test.Errorf("Registration failed for version 0: %s", err)
	}

	err = vt.AddVersion(1, st1{})
	if err != nil {
		test.Errorf("Registration failed for version 1: %s", err)
	}

	err = vt.AddVersion(0, st0{})
	if err == nil {
		test.Errorf("Re-registration succeeded for version 0")
	}	
}


func TestEncode(test *testing.T) {
	type st0 struct {
		Name string
	}

	var ts = NewTypeSet()
	var vt = ts.RegisterType("test")
	vt.AddVersion(0, st0{})

	var obj = &st0{ "Obj" }
	
	_, err := vt.Encode("one", obj)

	if err != nil {
		test.Errorf("Encoding error: %v", err)
	}

	type st1 struct {
		Name string
		Age uint16
	}

	var obj2 = &st1{ "Obj2", 2 }

	_, err = vt.Encode("two", obj2)

	if err == nil {
		test.Errorf("No encoding err when expected")
	}
}


