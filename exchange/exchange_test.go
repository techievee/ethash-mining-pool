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


	r := NewRestClient("Test", "https://api.coinmarketcap.com/v1/ticker/?convert=INR","15s")
	Result, err := r.GetData()

	if err != nil {
		t.Errorf("Error occured : %v", err)
		return
	}


  	for k,v := range Result  {

		fmt.Println("Key: %s , Value, %s", k,v)

	}



}

func BytesToString(data []byte) string {
	return string(data[:])
}