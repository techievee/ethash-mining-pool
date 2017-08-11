package exchange

import (

	"testing"
	"os"
	"fmt"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}


func TestGetData(t *testing.T){


	r := NewRestClient("Test", "https://ethexindia.com/api/ticker","15s")
	Result, err := r.GetData()

	if err != nil {
		t.Errorf("Error occured : %v", err)
		return
	}

	fmt.Println("Result:", Result)


}