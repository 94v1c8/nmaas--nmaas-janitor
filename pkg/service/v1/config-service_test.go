package v1

import "testing"

func TestCheckAPI(t *testing.T) {
	api := "wrong"
	current := "correct"
	err := checkAPI(api, current)
	if err == nil {
		t.Fail()
	}

	api = "correct"
	err = checkAPI(api, current)
	if err != nil {
		t.Fail()
	}
}