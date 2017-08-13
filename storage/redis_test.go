package storage

import (
	"os"
	"reflect"
	"strconv"
	"testing"

	"gopkg.in/redis.v3"

	"log"
)

var r *RedisClient

const prefix = "test"

func TestMain(m *testing.M) {
	r = NewRedisClient(&Config{Endpoint: "35.198.231.241:6379",Database: 1}, prefix,1000)
	reset()
	c := m.Run()
	reset()
	os.Exit(c)
}

func TestWriteShareCheckExist(t *testing.T) {
	reset()

	exist, _ := r.WriteShare("x", "x", []string{"0x0", "0x0", "0x0"}, 10, 1008, 0)
	if exist {
		t.Error("PoW must not exist")
	}
	exist, _ = r.WriteShare("x", "x", []string{"0x0", "0x1", "0x0"}, 10, 1008, 0)
	if exist {
		t.Error("PoW must not exist")
	}
	exist, _ = r.WriteShare("x", "x", []string{"0x0", "0x0", "0x1"}, 100, 1010, 0)
	if exist {
		t.Error("PoW must not exist")
	}
	exist, _ = r.WriteShare("z", "x", []string{"0x0", "0x0", "0x1"}, 100, 1016, 0)
	if !exist {
		t.Error("PoW must exist")
	}
	exist, _ = r.WriteShare("x", "x", []string{"0x0", "0x0", "0x1"}, 100, 1025, 0)
	if exist {
		t.Error("PoW must not exist")
	}
}

func TestGetPayees(t *testing.T) {
	reset()

	n := 256
	for i := 0; i < n; i++ {
		r.client.HSet(r.formatKey("miners", strconv.Itoa(i)), "balance", strconv.Itoa(i))
	}

	var payees []string
	payees, _ = r.GetPayees()
	if len(payees) != n {
		t.Error("Must return all payees")
	}
	m := make(map[string]struct{})
	for _, v := range payees {
		m[v] = struct{}{}
	}
	if len(m) != n {
		t.Error("Must be unique list")
	}
}

func TestGetBalance(t *testing.T) {
	reset()

	r.client.HSet(r.formatKey("miners:x"), "balance", "750")

	v, _ := r.GetBalance("x")
	if v != 750 {
		t.Error("Must return balance")
	}

	v, err := r.GetBalance("z")
	if v != 0 {
		t.Error("Must return 0 if account does not exist")
	}
	if err != nil {
		t.Error("Must not return error if account does not exist")
	}
}

func TestLockPayouts(t *testing.T) {
	reset()

	r.LockPayouts("x", 1000)
	v := r.client.Get("test:payments:lock").Val()
	if v != "x:1000" {
		t.Errorf("Invalid lock amount: %v", v)
	}

	err := r.LockPayouts("x", 100)
	if err == nil {
		t.Errorf("Must not overwrite lock")
	}
}

func TestUnlockPayouts(t *testing.T) {
	reset()

	r.client.Set(r.formatKey("payments:lock"), "x:1000", 0)

	r.UnlockPayouts()
	err := r.client.Get(r.formatKey("payments:lock")).Err()
	if err != redis.Nil {
		t.Errorf("Must release lock")
	}
}

func TestIsPayoutsLocked(t *testing.T) {
	reset()

	r.LockPayouts("x", 1000)
	if locked, _ := r.IsPayoutsLocked(); !locked {
		t.Errorf("Payouts must be locked")
	}
}

func TestUpdateBalance(t *testing.T) {
	reset()

	r.client.HMSetMap(
		r.formatKey("miners:x"),
		map[string]string{"paid": "50", "balance": "1000"},
	)
	r.client.HMSetMap(
		r.formatKey("finances"),
		map[string]string{"paid": "500", "balance": "10000"},
	)

	amount := int64(250)
	r.UpdateBalance("x", amount)
	result := r.client.HGetAllMap(r.formatKey("miners:x")).Val()
	if result["pending"] != "250" {
		t.Error("Must set pending amount")
	}
	if result["balance"] != "750" {
		t.Error("Must deduct balance")
	}
	if result["paid"] != "50" {
		t.Error("Must not touch paid")
	}

	result = r.client.HGetAllMap(r.formatKey("finances")).Val()
	if result["pending"] != "250" {
		t.Error("Must set pool pending amount")
	}
	if result["balance"] != "9750" {
		t.Error("Must deduct pool balance")
	}
	if result["paid"] != "500" {
		t.Error("Must not touch pool paid")
	}

	rank := r.client.ZRank(r.formatKey("payments:pending"), join("x", amount)).Val()
	if rank != 0 {
		t.Error("Must add pending payment")
	}
}

func TestRollbackBalance(t *testing.T) {
	reset()

	r.client.HMSetMap(
		r.formatKey("miners:x"),
		map[string]string{"paid": "100", "balance": "750", "pending": "250"},
	)
	r.client.HMSetMap(
		r.formatKey("finances"),
		map[string]string{"paid": "500", "balance": "10000", "pending": "250"},
	)
	r.client.ZAdd(r.formatKey("payments:pending"), redis.Z{Score: 1, Member: "xx"})

	amount := int64(250)
	r.RollbackBalance("x", amount)
	result := r.client.HGetAllMap(r.formatKey("miners:x")).Val()
	if result["paid"] != "100" {
		t.Error("Must not touch paid")
	}
	if result["balance"] != "1000" {
		t.Error("Must increase balance")
	}
	if result["pending"] != "0" {
		t.Error("Must deduct pending")
	}

	result = r.client.HGetAllMap(r.formatKey("finances")).Val()
	if result["paid"] != "500" {
		t.Error("Must not touch pool paid")
	}
	if result["balance"] != "10250" {
		t.Error("Must increase pool balance")
	}
	if result["pending"] != "0" {
		t.Error("Must deduct pool pending")
	}

	err := r.client.ZRank(r.formatKey("payments:pending"), join("x", amount)).Err()
	if err != redis.Nil {
		t.Errorf("Must remove pending payment")
	}
}

func TestWritePayment(t *testing.T) {
	reset()

	r.client.HMSetMap(
		r.formatKey("miners:x"),
		map[string]string{"paid": "50", "balance": "1000", "pending": "250"},
	)
	r.client.HMSetMap(
		r.formatKey("finances"),
		map[string]string{"paid": "500", "balance": "10000", "pending": "250"},
	)

	amount := int64(250)
	r.WritePayment("x", "0x0", amount)
	result := r.client.HGetAllMap(r.formatKey("miners:x")).Val()
	if result["pending"] != "0" {
		t.Error("Must unset pending amount")
	}
	if result["balance"] != "1000" {
		t.Error("Must not touch balance")
	}
	if result["paid"] != "300" {
		t.Error("Must increase paid")
	}

	result = r.client.HGetAllMap(r.formatKey("finances")).Val()
	if result["pending"] != "0" {
		t.Error("Must deduct pool pending amount")
	}
	if result["balance"] != "10000" {
		t.Error("Must not touch pool balance")
	}
	if result["paid"] != "750" {
		t.Error("Must increase pool paid")
	}

	err := r.client.Get(r.formatKey("payments:lock")).Err()
	if err != redis.Nil {
		t.Errorf("Must release lock")
	}

	err = r.client.ZRank(r.formatKey("payments:pending"), join("x", amount)).Err()
	if err != redis.Nil {
		t.Error("Must remove pending payment")
	}
	err = r.client.ZRank(r.formatKey("payments:all"), join("0x0", "x", amount)).Err()
	if err == redis.Nil {
		t.Error("Must add payment to set")
	}
	err = r.client.ZRank(r.formatKey("payments:x"), join("0x0", amount)).Err()
	if err == redis.Nil {
		t.Error("Must add payment to set")
	}
}

func TestGetPendingPayments(t *testing.T) {
	reset()

	r.client.HMSetMap(
		r.formatKey("miners:x"),
		map[string]string{"paid": "100", "balance": "750", "pending": "250"},
	)

	amount := int64(1000)
	r.UpdateBalance("x", amount)
	pending := r.GetPendingPayments()

	if len(pending) != 1 {
		t.Error("Must return pending payment")
	}
	if pending[0].Amount != amount {
		t.Error("Must have corrent amount")
	}
	if pending[0].Address != "x" {
		t.Error("Must have corrent account")
	}
	if pending[0].Timestamp <= 0 {
		t.Error("Must have timestamp")
	}
}

func TestCollectLuckStats(t *testing.T) {
	reset()

	members := []redis.Z{
		redis.Z{Score: 0, Member: "1:0:0x0:0x0:0:100:100:0"},
	}
	r.client.ZAdd(r.formatKey("blocks:immature"), members...)
	members = []redis.Z{
		redis.Z{Score: 1, Member: "1:0:0x2:0x0:0:50:100:0"},
		redis.Z{Score: 2, Member: "0:1:0x1:0x0:0:100:100:0"},
		redis.Z{Score: 3, Member: "0:0:0x3:0x0:0:200:100:0"},
	}
	r.client.ZAdd(r.formatKey("blocks:matured"), members...)

	stats, _ := r.CollectLuckStats([]int{1, 2, 5, 10})
	expectedStats := map[string]interface{}{
		"1": map[string]float64{
			"luck": 1, "uncleRate": 1, "orphanRate": 0,
		},
		"2": map[string]float64{
			"luck": 0.75, "uncleRate": 0.5, "orphanRate": 0,
		},
		"4": map[string]float64{
			"luck": 1.125, "uncleRate": 0.5, "orphanRate": 0.25,
		},
	}

	if !reflect.DeepEqual(stats, expectedStats) {
		t.Error("Stats != expected stats")
	}
}

func TestStoreExchangeData(t *testing.T) {


	m :=map[string]string{
		"id": "ethereum",
		"name": "Ethereum",
		"symbol": "ETH",
		"rank": "2",
		"price_usd": "311.984",
		"price_btc": "0.0823755",
		"24h_volume_usd": "1161280000.0",
		"market_cap_usd": "29309660622.0",
		"available_supply": "93946038.0",
		"total_supply": "93946038.0",
		"percent_change_1h": "0.47",
		"percent_change_24h": "4.12",
		"percent_change_7d": "30.36",
		"last_updated": "1502540048",
		"price_inr": "19995.366544",
		"24h_volume_inr": "74427596480.0",
		"market_cap_inr": "1878485458898",
	}
	m1 :=map[string]string{
		"id": "bitcoin",
		"name": "Bitcoin",
		"symbol": "BTC",
		"rank": "1",
		"price_usd": "3836.67",
		"price_btc": "1.0",
		"24h_volume_usd": "2080280000.0",
		"market_cap_usd": "63315651883.0",
		"available_supply": "16502762.0",
		"total_supply": "16502762.0",
		"percent_change_1h": "1.26",
		"percent_change_24h": "8.93",
		"percent_change_7d": "19.58",
		"last_updated": "1502551754",
		"price_inr": "245896.01697",
		"24h_volume_inr": "133327225479.9999847412",
		"market_cap_inr": "4057963444804",
	}



	data :=[]map[string]string{
		m1,
		m,
	}



	tx := r.client.Multi()
	defer tx.Close()

	for _,v := range data  {

		for k1,v1 := range v{
				tx.HSet(r.formatKey("exchange", v["symbol"]),k1,v1)
		}
	}
	log.Print("Writing Exchange Data : %v",data)
}

func TestGetExchangeData(t *testing.T) {

	cmd := r.client.HGetAllMap(r.formatKey("exchange", "ETH" ))
	result,err := cmd.Result()

	log.Printf("Writing Exchange Data : %v ",result)

	if err!=nil{
		t.Errorf("Error at GetExchangeData:",err)
	}


}

func reset() {
	keys := r.client.Keys(r.prefix + ":*").Val()
	for _, k := range keys {
		r.client.Del(k)
	}
}


