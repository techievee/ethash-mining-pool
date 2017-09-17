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
	r = NewRedisClient(&Config{Endpoint: "35.187.240.179:6379",Database: 10}, prefix,1000000, "UBIQ")
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
	tx := int64(2)
	r.WritePayment("x", "0x0", amount, tx)
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


func TestCollectStats(t *testing.T) {
	stat, err := r.CollectStats(500000, 100, 100)


	if err!=nil{
		t.Errorf("Result : %v, Err : %v", stat,err)
	}
	t.Logf("Result : %v", stat)
}

func TestGetMinerStats(t *testing.T){


	stats := make(map[string]interface{})

	login := "0x5ca87a9e8e132be404a1efb6516665252a74a4e2"


	tx := r.client.Multi()
	defer tx.Close()

	cmds, err := tx.Exec(func() error {
		tx.HGetAllMap(r.formatKey("miners", login))
		tx.ZRevRangeWithScores(r.formatKey("payments", login), 0, 100)
		tx.ZCard(r.formatKey("payments", login))
		tx.HGet(r.formatKey("shares", "roundCurrent"), login)
		return nil
	})

	if err != nil && err != redis.Nil {
		t.Errorf("Error :",err)
	} else {
		result, _ := cmds[0].(*redis.StringStringMapCmd).Result()
		stats["stats"] = convertStringMap(result)
		payments := convertPaymentsResults(cmds[1].(*redis.ZSliceCmd))
		stats["payments"] = payments
		stats["paymentsTotal"] = cmds[2].(*redis.IntCmd).Val()
		roundShares, _ := cmds[3].(*redis.StringCmd).Int64()
		if roundShares < 0 {
			roundShares = 0
		}
		stats["roundShares"] = roundShares
		log.Printf("Inner Result : %v ",result)
	}


	log.Printf("Result : %v ",stats)

	if err!=nil{
		t.Errorf("Error :",err)
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


func TestCreateNewNValue(t *testing.T) {

	result, err := r.CreateNewNValue(4000000000)
	if err!=nil{
		t.Errorf("Result : %v, Err : %v", result,err)
	}
	t.Logf("Result : %v", result)
}

func TestGetNetworkDifficultyForCurrentShareDifficulty(t *testing.T) {

	//m ,err  := r.GetNodeStates()
	result, err := r.GetNetworkDifficultyForCurrentShareDifficulty(4000000000)
	if err!=nil{
		t.Errorf("Result : %v, Err : %v", result, err)
	}
	t.Logf("Result : %v", result)

}


func TestGetNetworkDifficulty(t *testing.T) {
	result,err := r.GetNetworkDifficulty()
	if err!=nil{
		t.Errorf("Result : %v, Err :%v", result, err)
	}
	t.Logf("Result : %v", result)

}


func TestGetThreshold(t *testing.T){
	result, err := r.SetThreshold("0xfacb288273969c68e9ad1eeeb81f08ab92cf57ad",5000000)
	t.Logf("Result : %v",result)
	if err!=nil{
		t.Errorf("Error , %v",err)
	}
}

func TestSetThreshold(t *testing.T){
	r.SetThreshold("0xfacb288273969c68e9ad1eeeb81f08ab92cf57ad",5000000)
	result, err := r.GetThreshold("0xfacb288273969c68e9ad1eeeb81f08ab92cf57ad")
	t.Logf("Result : %v",result)
	if err!=nil{
		t.Errorf("Error , %v",err)
	}

}


func TestLogIP(t *testing.T){

	r.LogIP("0xb9cf2da90bdff1bc014720cc84f5ab99d7974eba","192.168.00.100")

}

func TestAdjustCurrentNShares(t *testing.T) {

	result,err := r.AdjustCurrentNShares(4000000000)
	t.Logf("Result : %v",result)
	if err!=nil{
		t.Errorf("Error , %v",err)
	}


	/*currentNShare := 1010
	lastN := 1000


	tx := r.client.Multi()
	defer tx.Close()


	if currentNShare > lastN{


		shareHash := make([]string, currentNShare-lastN)

		cmd, err := tx.Exec(func() error {

			//Keep removing the shares from the List by RPOP and while removing adjust the correcponding miner share value and the stat:roundCurrent Share value
			//count :=0

			for loopIndex := currentNShare; loopIndex > lastN; loopIndex--{

				//Generate all the poped value of the ShareHash on the Array
				//tx.LIndex(r.formatKey("lastshares"),-1)
				tx.RPop(r.formatKey("lastshares"))

				//tx.HIncrBy(r.formatKey("shares", "roundCurrent"), str, -1)

				//t.Logf("List index value : %v", str)
				//count++

			}
			return nil
		})
		if err != nil {
			t.Logf("Error while Reducing the share count , %v", err)
		} else {

			tx2 := r.client.Multi()
			defer tx2.Close()

			//Decrement the corresponding share value
			_, err := tx2.Exec(func() error {
				for key , _ := range shareHash {
					poppedValue, err := cmd[key].(*redis.StringCmd).Result()
					//poppedValue1, err := cmd[1].(*redis.StringCmd).Result()
					if err==nil{
						tx2.HIncrBy(r.formatKey("stats"), "roundShares", -1)
						tx2.HIncrBy(r.formatKey("shares", "roundCurrent"),poppedValue, -1)
						return errors.New("TEST RETURN")
					}
					log.Print(poppedValue)
					log.Print(key)
					//log.Print(poppedValue1)
				}
				return nil
			})
			if err!=nil{
				t.Errorf("Error while adjusting the last share window count , %v", err)

			}
		}

	} else {
		//No adjustment is required for the Window
		t.Logf("No formatting required")
	}
*/


}


func TestWriteBlock(t *testing.T) {

}


func TestWriteShare(t *testing.T) {

}

func reset() {
	keys := r.client.Keys(r.prefix + ":*").Val()
	for _, k := range keys {
		r.client.Del(k)
	}
}


