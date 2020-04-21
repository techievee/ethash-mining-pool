package storage

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/techievee/ethash-mining-pool/util"
	"gopkg.in/redis.v3"
	"log"
)

type Config struct {
	Endpoint string `json:"endpoint"`
	Password string `json:"password"`
	Database int64  `json:"database"`
	PoolSize int    `json:"poolSize"`
}

type RedisClient struct {
	client   *redis.Client
	prefix   string
	pplns    int64
	CoinName string
}
type SumRewardData struct {
	Interval int64  `json:"inverval"`
	Reward   int64  `json:"reward"`
	Name     string `json:"name"`
	Offset   int64  `json:"offset"`
}
type RewardData struct {
	Height    int64   `json:"blockheight"`
	Timestamp int64   `json:"timestamp"`
	BlockHash string  `json:"blockhash"`
	Reward    int64   `json:"reward"`
	Percent   float64 `json:"percent"`
	Immature  bool    `json:"immature"`
}
type BlockData struct {
	Height         int64    `json:"height"`
	Timestamp      int64    `json:"timestamp"`
	Difficulty     int64    `json:"difficulty"`
	TotalShares    int64    `json:"shares"`
	Uncle          bool     `json:"uncle"`
	UncleHeight    int64    `json:"uncleHeight"`
	Orphan         bool     `json:"orphan"`
	Hash           string   `json:"hash"`
	Nonce          string   `json:"-"`
	PowHash        string   `json:"-"`
	MixDigest      string   `json:"-"`
	Reward         *big.Int `json:"-"`
	ExtraReward    *big.Int `json:"-"`
	ImmatureReward string   `json:"-"`
	RewardString   string   `json:"reward"`
	RoundHeight    int64    `json:"-"`
	candidateKey   string
	immatureKey    string
}

func (b *BlockData) RewardInShannon() int64 {
	reward := new(big.Int).Div(b.Reward, util.Shannon)
	return reward.Int64()
}

func (b *BlockData) serializeHash() string {
	if len(b.Hash) > 0 {
		return b.Hash
	} else {
		return "0x0"
	}
}

func (b *BlockData) RoundKey() string {
	return join(b.RoundHeight, b.Hash)
}

func (b *BlockData) key() string {
	return join(b.UncleHeight, b.Orphan, b.Nonce, b.serializeHash(), b.Timestamp, b.Difficulty, b.TotalShares, b.Reward)
}

type Miner struct {
	LastBeat  int64 `json:"lastBeat"`
	HR        int64 `json:"hr"`
	Offline   bool  `json:"offline"`
	startedAt int64
}

type Worker struct {
	Miner
	TotalHR int64 `json:"hr2"`
}

func NewRedisClient(cfg *Config, prefix string, pplns int64, CoinName string) *RedisClient {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Endpoint,
		Password: cfg.Password,
		DB:       cfg.Database,
		PoolSize: cfg.PoolSize,
	})
	return &RedisClient{client: client, prefix: prefix, pplns: pplns, CoinName: CoinName}
}

func (r *RedisClient) Client() *redis.Client {
	return r.client
}

func (r *RedisClient) Check() (string, error) {
	return r.client.Ping().Result()
}

func (r *RedisClient) BgSave() (string, error) {
	return r.client.BgSave().Result()
}

// Always returns list of addresses. If Redis fails it will return empty list.
func (r *RedisClient) GetBlacklist() ([]string, error) {
	cmd := r.client.SMembers(r.formatKey("blacklist"))
	if cmd.Err() != nil {
		return []string{}, cmd.Err()
	}
	return cmd.Val(), nil
}

// Always returns list of IPs. If Redis fails it will return empty list.
func (r *RedisClient) GetWhitelist() ([]string, error) {
	cmd := r.client.SMembers(r.formatKey("whitelist"))
	if cmd.Err() != nil {
		return []string{}, cmd.Err()
	}
	return cmd.Val(), nil
}

func (r *RedisClient) WriteNodeState(id string, height uint64, diff *big.Int) error {
	tx := r.client.Multi()
	defer tx.Close()

	now := util.MakeTimestamp() / 1000

	_, err := tx.Exec(func() error {
		tx.HSet(r.formatKey("nodes"), join(id, "name"), id)
		tx.HSet(r.formatKey("nodes"), join(id, "height"), strconv.FormatUint(height, 10))
		tx.HSet(r.formatKey("nodes"), join(id, "difficulty"), diff.String())
		tx.HSet(r.formatKey("nodes"), join(id, "lastBeat"), strconv.FormatInt(now, 10))
		return nil
	})
	return err
}

func (r *RedisClient) GetNodeStates() ([]map[string]interface{}, error) {
	cmd := r.client.HGetAllMap(r.formatKey("nodes"))
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	m := make(map[string]map[string]interface{})
	for key, value := range cmd.Val() {
		parts := strings.Split(key, ":")
		if val, ok := m[parts[0]]; ok {
			val[parts[1]] = value
		} else {
			node := make(map[string]interface{})
			node[parts[1]] = value
			m[parts[0]] = node
		}
	}
	v := make([]map[string]interface{}, len(m), len(m))
	i := 0
	for _, value := range m {
		v[i] = value
		i++
	}
	return v, nil
}

func (r *RedisClient) checkPoWExist(height uint64, params []string) (bool, error) {
	// Sweep PoW backlog for previous blocks, we have 3 templates back in RAM
	r.client.ZRemRangeByScore(r.formatKey("pow"), "-inf", fmt.Sprint("(", height-8))
	val, err := r.client.ZAdd(r.formatKey("pow"), redis.Z{Score: float64(height), Member: strings.Join(params, ":")}).Result()
	return val == 0, err
}

func (r *RedisClient) WriteShare(login, id string, params []string, diff int64, height uint64, window time.Duration) (bool, error) {

	exist, err := r.checkPoWExist(height, params)
	if err != nil {
		return false, err
	}
	// Duplicate share, (nonce, powHash, mixDigest) pair exist
	if exist {
		return true, nil
	}
	tx := r.client.Multi()
	defer tx.Close()

	ms := util.MakeTimestamp()
	ts := ms / 1000

	_, err = tx.Exec(func() error {
		r.writeShare(tx, ms, ts, login, id, diff, window)
		return nil
	})

	adj, err1 := r.AdjustCurrentNShares(diff)

	if err != nil || adj == false {
		log.Printf("Error while adjusting share windows while writing share: %s ", err1)
	}

	return false, err
}

func (r *RedisClient) AdjustCurrentNShares(diff int64) (bool, error) {

	//Get the current Value of N from the redis, which was pre calulculatee based on the blockchain difficulty * 2
	lastN, err := r.GetLastNValue() //O(1)
	if err != nil || lastN == 0 {
		lastN, err = r.CreateNewNValue(diff) //o(c)
		if err != nil || lastN == 0 {
			lastN = r.pplns
			_, err := r.client.HSet(r.formatKey("stats"), "lastNValue", strconv.FormatInt(r.pplns, 10)).Result()
			log.Print("Error while generating new lastN value.Fixing N value as pplns value %v, Error: %v", r.pplns, err)
		}
	}

	//Check the Number of list content of lastShares to find, current number of list present
	currentNShare := r.client.LLen(r.formatKey("lastshares")).Val()

	tx := r.client.Multi()
	defer tx.Close()

	//CurrentNumber of Shares in the list is more thatn the actual N value for this round
	//ie, If number is more than the blockchaindifficulty*2, the remove those numbers and reduce the lastnshares from the miners context
	if currentNShare > lastN {

		cmd, err := tx.Exec(func() error {

			//Keep removing the shares from the List by RPOP and while removing adjust the correcponding miner share value and the stat:roundCurrent Share value
			for loopIndex := currentNShare; loopIndex > lastN; loopIndex-- {
				//Pop the value and we will use the cmd to get the values that has been popped out
				tx.RPop(r.formatKey("lastshares"))
			}
			return nil
		})
		if err != nil {
			log.Print("Error while Popping the share count returning without adjustment , %v", err)
			return false, err
		} else {
			tx2 := r.client.Multi()
			defer tx2.Close()

			//Decrement the corresponding share value of the popped shares
			_, err := tx2.Exec(func() error {
				count := 0
				for loopIndex := currentNShare; loopIndex > lastN; loopIndex-- {
					poppedValue, err := cmd[count].(*redis.StringCmd).Result()
					count++
					if err == nil {
						tx2.HIncrBy(r.formatKey("stats"), "roundShares", -1)
						tx2.HIncrBy(r.formatKey("shares", "roundCurrent"), poppedValue, -1)
					} else {
						log.Printf("Error while Decrementing the share windows, continuing , Error:  %v", err)
					}
				}
				return nil
			})

			if err != nil {
				log.Print("Error while adjusting the last share window count , %v", err)
				return false, err
			} else {
				return true, nil
			}
		} //Else part, Success of first transaction

	} else {
		//No adjustment is required for the Window
		return true, nil
	}

	return false, err

}

func (r *RedisClient) GetNetworkDifficulty() (*big.Int, error) {

	NetworkDifficultyDivShareDiff := big.NewInt(0)
	m, err := r.GetNodeStates()
	if err != nil {
		return NetworkDifficultyDivShareDiff, err

	}
	for _, value := range m {
		for legend, data := range value {
			if legend == "difficulty" {
				NetworkDifficultyDivShareDiff.SetString(join(data), 10)
				return NetworkDifficultyDivShareDiff, nil
			}
		}
	}

	return NetworkDifficultyDivShareDiff, err

}

func (r *RedisClient) GetNetworkDifficultyForCurrentShareDifficulty(sharediff int64) (int64, error) {

	NetworkDifficulty := big.NewInt(0)
	NetworkDifficultyDivShareDiff := big.NewInt(0)
	NetworkDifficulty, err := r.GetNetworkDifficulty()

	if err == nil {
		NetworkDifficultyDivShareDiff.Div(NetworkDifficulty, big.NewInt(sharediff))
		return NetworkDifficultyDivShareDiff.Int64(), err
	}

	return NetworkDifficultyDivShareDiff.Int64(), nil

}

func (r *RedisClient) CreateNewNValue(shareDiff int64) (int64, error) {

	newlastN, err := r.GetNetworkDifficultyForCurrentShareDifficulty(shareDiff)
	if err != nil {
		log.Print("Error while Get the Network difficulty for the current share, %v", err)
		return newlastN, err
	}

	//2 time blockchain difficulty for the Share value currentDiff/sharediff * 2 [PPLNS Window Size]
	newlastN = newlastN * 2

	cmd := r.client.HSet(r.formatKey("stats"), "lastNValue", strconv.FormatInt(newlastN, 10))
	_, err = cmd.Result()
	if err != nil {
		log.Print("Error while sotring the newly generated last N Value, %v", err)
		return newlastN, err
	}
	return newlastN, err

}

func (r *RedisClient) LogIP(login string, ip string) {

	r.client.HSet(r.formatKey("settings", login), "ip_addr", ip)
	ms := util.MakeTimestamp()
	ts := ms / 1000
	r.client.HSet(r.formatKey("settings", login), "ip_time", strconv.FormatInt(ts, 10))

}

func (r *RedisClient) GetLastNValue() (int64, error) {

	val := int64(0)
	cmd := r.client.HGet(r.formatKey("stats"), "lastNValue")
	val, err := cmd.Int64()
	if err != nil {
		return val, err
	}

	return val, err

}

func (r *RedisClient) WriteBlock(login, id string, params []string, diff, roundDiff int64, height uint64, window time.Duration) (bool, error) {
	exist, err := r.checkPoWExist(height, params)
	if err != nil {
		return false, err
	}
	// Duplicate share, (nonce, powHash, mixDigest) pair exist
	if exist {
		return true, nil
	}
	tx := r.client.Multi()
	defer tx.Close()

	ms := util.MakeTimestamp()
	ts := ms / 1000

	totalShares := int64(0)

	cmds, err := tx.Exec(func() error {
		r.writeShare(tx, ms, ts, login, id, diff, window)
		tx.HSet(r.formatKey("stats"), "lastBlockFound", strconv.FormatInt(ts, 10))
		tx.HGet(r.formatKey("stats"), "currentRoundShares")
		tx.HDel(r.formatKey("stats"), "currentRoundShares")
		tx.ZIncrBy(r.formatKey("finders"), 1, login)
		tx.HIncrBy(r.formatKey("miners", login), "blocksFound", 1)
		tx.HGetAllMap(r.formatKey("shares", "roundCurrent"))
		return nil
	})
	if err != nil {
		return false, err
	} else {

		//We are not going to calculate the shares from the list calculation,we already have the values of totalShare in the stat:roundShares and the miners share in the shares:roundCurrent:login - Value
		tx2 := r.client.Multi()
		defer tx2.Close()

		//Iterate on the roundCurrent Values shares:roundCurrent:login and store the Share values for the block which was found
		_, err := tx2.Exec(func() error {
			//we use negative indexing since we have writeshare fn with variable number of commands
			sharesMap, _ := cmds[len(cmds)-1].(*redis.StringStringMapCmd).Result()

			for k, v := range sharesMap {
				n, _ := strconv.ParseInt(v, 10, 64)
				//Now we will store the Share Count , Params[0] contains the nonce of the found block
				tx2.HIncrBy(r.formatRound(int64(height), params[0]), k, n)
			}

			//when the round is over we can get the current diffuclty using these values
			totalShares, _ = cmds[len(cmds)-5].(*redis.StringCmd).Int64()
			//Just store the difficulty of the block by X by diff
			totalShares = totalShares * diff
			return nil
		})
		if err != nil {
			return false, err
		}

		hashHex := strings.Join(params, ":")
		s := join(hashHex, ts, roundDiff, totalShares)
		cmd := r.client.ZAdd(r.formatKey("blocks", "candidates"), redis.Z{Score: float64(height), Member: s})

		//Calulcate the New 'N' Value  from the current network diffuclty and Store the N the stat table for the Next Round
		lastN := int64(0)
		lastN, err = r.CreateNewNValue(diff)
		if err != nil {
			log.Printf("Error while Creating a new N Value, we will continue with the old values, Error : %v", err)
			return false, err

		} else {
			log.Printf("New N Value is generated for the block : %v", lastN)
		}
		//Call the Function to adjust the List window according to the current value of N, Remove the RPOP and Adjust reduce the round and total share accourdingly
		adj, err := r.AdjustCurrentNShares(diff)

		if err != nil || adj == false {
			log.Printf("Error while adjusting new block : %s ", err)
			return false, err
		} else {
			return false, nil
		}

		return false, cmd.Err()
	}
}

func (r *RedisClient) writeShare(tx *redis.Multi, ms, ts int64, login, id string, diff int64, expire time.Duration) {
	tx.LPush(r.formatKey("lastshares"), login)
	//Not required as we RPOP later to adjust the window, when the round completes
	//tx.LTrim(r.formatKey("lastshares"), 0, r.pplns)

	//Calculate the total difficulty of the current round
	tx.HIncrBy(r.formatKey("stats"), "currentRoundShares", 1)

	//Used in calculating the roundPercent, shares:roundCurrent:login / stats:roundShares, Shares is always maintained in hashes to cope with future change in share difficulty
	tx.HIncrBy(r.formatKey("shares", "roundCurrent"), login, 1)
	//Contains the total shares that had been contributed for the round, and it is used while calculating the percentage, based on the current share value
	tx.HIncrBy(r.formatKey("stats"), "roundShares", 1)

	tx.ZAdd(r.formatKey("hashrate"), redis.Z{Score: float64(ts), Member: join(diff, login, id, ms)})
	tx.ZAdd(r.formatKey("hashrate", login), redis.Z{Score: float64(ts), Member: join(diff, id, ms)})
	tx.Expire(r.formatKey("hashrate", login), expire) // Will delete hashrates for miners that gone
	tx.HSet(r.formatKey("miners", login), "lastShare", strconv.FormatInt(ts, 10))
}

func (r *RedisClient) formatKey(args ...interface{}) string {
	return join(r.prefix, join(args...))
}

func (r *RedisClient) formatRound(height int64, nonce string) string {
	return r.formatKey("shares", "round"+strconv.FormatInt(height, 10), nonce)
}

func join(args ...interface{}) string {
	s := make([]string, len(args))
	for i, v := range args {
		switch v.(type) {
		case string:
			s[i] = v.(string)
		case int64:
			s[i] = strconv.FormatInt(v.(int64), 10)
		case uint64:
			s[i] = strconv.FormatUint(v.(uint64), 10)
		case float64:
			s[i] = strconv.FormatFloat(v.(float64), 'f', 0, 64)
		case bool:
			if v.(bool) {
				s[i] = "1"
			} else {
				s[i] = "0"
			}
		case *big.Int:
			n := v.(*big.Int)
			if n != nil {
				s[i] = n.String()
			} else {
				s[i] = "0"
			}
		case *big.Rat:
			x := v.(*big.Rat)
			if x != nil {
				s[i] = x.FloatString(9)
			} else {
				s[i] = "0"
			}
		default:
			panic("Invalid type specified for conversion")
		}
	}
	return strings.Join(s, ":")
}

func (r *RedisClient) GetCandidates(maxHeight int64) ([]*BlockData, error) {
	option := redis.ZRangeByScore{Min: "0", Max: strconv.FormatInt(maxHeight, 10)}
	cmd := r.client.ZRangeByScoreWithScores(r.formatKey("blocks", "candidates"), option)
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	return convertCandidateResults(cmd), nil
}

func (r *RedisClient) GetImmatureBlocks(maxHeight int64) ([]*BlockData, error) {
	option := redis.ZRangeByScore{Min: "0", Max: strconv.FormatInt(maxHeight, 10)}
	cmd := r.client.ZRangeByScoreWithScores(r.formatKey("blocks", "immature"), option)
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	return convertBlockResults(cmd), nil
}
func (r *RedisClient) GetRewards(login string) ([]*RewardData, error) {
	option := redis.ZRangeByScore{Min: "0", Max: strconv.FormatInt(10, 10)}
	cmd := r.client.ZRangeByScoreWithScores(r.formatKey("rewards", login), option)
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	return convertRewardResults(cmd), nil
}

func (r *RedisClient) GetRoundShares(height int64, nonce string) (map[string]int64, error) {
	result := make(map[string]int64)
	cmd := r.client.HGetAllMap(r.formatRound(height, nonce))
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	sharesMap, _ := cmd.Result()
	for login, v := range sharesMap {
		n, _ := strconv.ParseInt(v, 10, 64)
		result[login] = n
	}
	return result, nil
}

func (r *RedisClient) GetPayees() ([]string, error) {
	payees := make(map[string]struct{})
	var result []string
	var c int64

	for {
		var keys []string
		var err error
		c, keys, err = r.client.Scan(c, r.formatKey("miners", "*"), 100).Result()
		if err != nil {
			return nil, err
		}
		for _, row := range keys {
			login := strings.Split(row, ":")[2]
			payees[login] = struct{}{}
		}
		if c == 0 {
			break
		}
	}
	for login, _ := range payees {
		result = append(result, login)
	}
	return result, nil
}
func (r *RedisClient) GetTotalShares() (int64, error) {
	cmd := r.client.LLen(r.formatKey("lastshares"))
	if cmd.Err() == redis.Nil {
		return 0, nil
	} else if cmd.Err() != nil {
		return 0, cmd.Err()
	}
	return cmd.Val(), nil
}

func (r *RedisClient) GetBalance(login string) (int64, error) {
	cmd := r.client.HGet(r.formatKey("miners", login), "balance")
	if cmd.Err() == redis.Nil {
		return 0, nil
	} else if cmd.Err() != nil {
		return 0, cmd.Err()
	}
	return cmd.Int64()
}

func (r *RedisClient) GetThreshold(login string) (int64, error) {
	cmd := r.client.HGet(r.formatKey("settings", login), "payoutthreshold")
	if cmd.Err() == redis.Nil {
		return 0, nil
	} else if cmd.Err() != nil {
		return 0, cmd.Err()
	}
	return cmd.Int64()
}

func (r *RedisClient) SetThreshold(login string, threshold int64) (bool, error) {
	cmd, err := r.client.HSet(r.formatKey("settings", login), "payoutthreshold", strconv.FormatInt(threshold, 10)).Result()
	return cmd, err
}

func (r *RedisClient) LockPayouts(login string, amount int64) error {
	key := r.formatKey("payments", "lock")
	result := r.client.SetNX(key, join(login, amount), 0).Val()
	if !result {
		return fmt.Errorf("Unable to acquire lock '%s'", key)
	}
	return nil
}

func (r *RedisClient) UnlockPayouts() error {
	key := r.formatKey("payments", "lock")
	_, err := r.client.Del(key).Result()
	return err
}

func (r *RedisClient) IsPayoutsLocked() (bool, error) {
	_, err := r.client.Get(r.formatKey("payments", "lock")).Result()
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return true, nil
	}
}

type PendingPayment struct {
	Timestamp int64  `json:"timestamp"`
	Amount    int64  `json:"amount"`
	Address   string `json:"login"`
}

func (r *RedisClient) GetPendingPayments() []*PendingPayment {
	raw := r.client.ZRevRangeWithScores(r.formatKey("payments", "pending"), 0, -1)
	var result []*PendingPayment
	for _, v := range raw.Val() {
		// timestamp -> "address:amount"
		payment := PendingPayment{}
		payment.Timestamp = int64(v.Score)
		fields := strings.Split(v.Member.(string), ":")
		payment.Address = fields[0]
		payment.Amount, _ = strconv.ParseInt(fields[1], 10, 64)
		result = append(result, &payment)
	}
	return result
}

// Deduct miner's balance for payment
func (r *RedisClient) UpdateBalance(login string, amount int64) error {
	tx := r.client.Multi()
	defer tx.Close()

	ts := util.MakeTimestamp() / 1000

	_, err := tx.Exec(func() error {
		tx.HIncrBy(r.formatKey("miners", login), "balance", (amount * -1))
		tx.HIncrBy(r.formatKey("miners", login), "pending", amount)
		tx.HIncrBy(r.formatKey("finances"), "balance", (amount * -1))
		tx.HIncrBy(r.formatKey("finances"), "pending", amount)
		tx.ZAdd(r.formatKey("payments", "pending"), redis.Z{Score: float64(ts), Member: join(login, amount)})
		return nil
	})
	return err
}

func (r *RedisClient) RollbackBalance(login string, amount int64) error {
	tx := r.client.Multi()
	defer tx.Close()

	_, err := tx.Exec(func() error {
		tx.HIncrBy(r.formatKey("miners", login), "balance", amount)
		tx.HIncrBy(r.formatKey("miners", login), "pending", (amount * -1))
		tx.HIncrBy(r.formatKey("finances"), "balance", amount)
		tx.HIncrBy(r.formatKey("finances"), "pending", (amount * -1))
		tx.ZRem(r.formatKey("payments", "pending"), join(login, amount))
		return nil
	})
	return err
}

func (r *RedisClient) WritePayment(login, txHash string, amount int64, txCharges int64) error {
	tx := r.client.Multi()
	defer tx.Close()

	ts := util.MakeTimestamp() / 1000

	_, err := tx.Exec(func() error {
		tx.HIncrBy(r.formatKey("miners", login), "pending", (amount * -1))
		tx.HIncrBy(r.formatKey("miners", login), "paid", amount)
		tx.HIncrBy(r.formatKey("finances"), "pending", (amount * -1))
		tx.HIncrBy(r.formatKey("finances"), "paid", amount)
		tx.HIncrBy(r.formatKey("finances"), "txcharges", txCharges)

		tx.ZAdd(r.formatKey("payments", "all"), redis.Z{Score: float64(ts), Member: join(txHash, login, amount, txCharges)})
		tx.ZAdd(r.formatKey("payments", login), redis.Z{Score: float64(ts), Member: join(txHash, amount, txCharges)})
		tx.ZRem(r.formatKey("payments", "pending"), join(login, amount))
		tx.Del(r.formatKey("payments", "lock"))
		return nil
	})
	return err
}
func (r *RedisClient) WriteReward(login string, amount int64, percent *big.Rat, immature bool, block *BlockData) error {
	if amount <= 0 {
		return nil
	}
	tx := r.client.Multi()
	defer tx.Close()

	addStr := join(amount, percent, immature, block.Hash, block.Height, block.Timestamp)
	remStr := join(amount, percent, !immature, block.Hash, block.Height, block.Timestamp)

	remscore := block.Timestamp - 3600*24*90 // Store the last 90 Days

	_, err := tx.Exec(func() error {
		tx.ZAdd(r.formatKey("rewards", login), redis.Z{Score: float64(block.Timestamp), Member: addStr})
		tx.ZRem(r.formatKey("rewards", login), remStr)
		tx.ZRemRangeByScore(r.formatKey("rewards", login), "-inf", "("+strconv.FormatInt(remscore, 10))
		return nil
	})
	return err
}

func (r *RedisClient) WriteImmatureBlock(block *BlockData, roundRewards map[string]int64) error {
	tx := r.client.Multi()
	defer tx.Close()

	_, err := tx.Exec(func() error {
		r.writeImmatureBlock(tx, block)
		total := int64(0)
		for login, amount := range roundRewards {
			total += amount
			tx.HIncrBy(r.formatKey("miners", login), "immature", amount)
			tx.HSetNX(r.formatKey("credits", "immature", block.Height, block.Hash), login, strconv.FormatInt(amount, 10))
		}
		tx.HIncrBy(r.formatKey("finances"), "immature", total)
		return nil
	})
	return err
}

func (r *RedisClient) WriteMaturedBlock(block *BlockData, roundRewards map[string]int64) error {
	creditKey := r.formatKey("credits", "immature", block.RoundHeight, block.Hash)
	tx, err := r.client.Watch(creditKey)
	// Must decrement immatures using existing log entry
	immatureCredits := tx.HGetAllMap(creditKey)
	if err != nil {
		return err
	}
	defer tx.Close()

	ts := util.MakeTimestamp() / 1000
	value := join(block.Hash, ts, block.Reward)

	_, err = tx.Exec(func() error {
		r.writeMaturedBlock(tx, block)
		tx.ZAdd(r.formatKey("credits", "all"), redis.Z{Score: float64(block.Height), Member: value})

		// Decrement immature balances
		totalImmature := int64(0)
		for login, amountString := range immatureCredits.Val() {
			amount, _ := strconv.ParseInt(amountString, 10, 64)
			totalImmature += amount
			tx.HIncrBy(r.formatKey("miners", login), "immature", (amount * -1))
		}

		// Increment balances
		total := int64(0)
		for login, amount := range roundRewards {
			total += amount
			// NOTICE: Maybe expire round reward entry in 604800 (a week)?
			tx.HIncrBy(r.formatKey("miners", login), "balance", amount)
			if amount > 0 {
				tx.HSetNX(r.formatKey("credits", block.Height, block.Hash), login, strconv.FormatInt(amount, 10))
			}

		}
		tx.Del(creditKey)
		tx.HIncrBy(r.formatKey("finances"), "balance", total)
		tx.HIncrBy(r.formatKey("finances"), "immature", (totalImmature * -1))
		tx.HSet(r.formatKey("finances"), "lastCreditHeight", strconv.FormatInt(block.Height, 10))
		tx.HSet(r.formatKey("finances"), "lastCreditHash", block.Hash)
		tx.HIncrBy(r.formatKey("finances"), "totalMined", block.RewardInShannon())
		return nil
	})
	return err
}

func (r *RedisClient) WriteOrphan(block *BlockData) error {
	creditKey := r.formatKey("credits", "immature", block.RoundHeight, block.Hash)
	tx, err := r.client.Watch(creditKey)
	// Must decrement immatures using existing log entry
	immatureCredits := tx.HGetAllMap(creditKey)
	if err != nil {
		return err
	}
	defer tx.Close()

	_, err = tx.Exec(func() error {
		r.writeMaturedBlock(tx, block)

		// Decrement immature balances
		totalImmature := int64(0)
		for login, amountString := range immatureCredits.Val() {
			amount, _ := strconv.ParseInt(amountString, 10, 64)
			totalImmature += amount
			tx.HIncrBy(r.formatKey("miners", login), "immature", (amount * -1))
		}
		tx.Del(creditKey)
		tx.HIncrBy(r.formatKey("finances"), "immature", (totalImmature * -1))
		return nil
	})
	return err
}

func (r *RedisClient) WritePendingOrphans(blocks []*BlockData) error {
	tx := r.client.Multi()
	defer tx.Close()

	_, err := tx.Exec(func() error {
		for _, block := range blocks {
			r.writeImmatureBlock(tx, block)
		}
		return nil
	})
	return err
}

func (r *RedisClient) writeImmatureBlock(tx *redis.Multi, block *BlockData) {
	// Redis 2.8.x returns "ERR source and destination objects are the same"
	if block.Height != block.RoundHeight {
		tx.Rename(r.formatRound(block.RoundHeight, block.Nonce), r.formatRound(block.Height, block.Nonce))
	}
	tx.ZRem(r.formatKey("blocks", "candidates"), block.candidateKey)
	tx.ZAdd(r.formatKey("blocks", "immature"), redis.Z{Score: float64(block.Height), Member: block.key()})
}

func (r *RedisClient) writeMaturedBlock(tx *redis.Multi, block *BlockData) {
	tx.Del(r.formatRound(block.RoundHeight, block.Nonce))
	tx.ZRem(r.formatKey("blocks", "immature"), block.immatureKey)
	tx.ZAdd(r.formatKey("blocks", "matured"), redis.Z{Score: float64(block.Height), Member: block.key()})
}

func (r *RedisClient) IsMinerExists(login string) (bool, error) {
	return r.client.Exists(r.formatKey("miners", login)).Result()
}

func (r *RedisClient) GetMinerStats(login string, maxPayments int64) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	tx := r.client.Multi()
	defer tx.Close()

	cmds, err := tx.Exec(func() error {
		tx.HGetAllMap(r.formatKey("miners", login))
		tx.ZRevRangeWithScores(r.formatKey("payments", login), 0, maxPayments-1)
		tx.ZCard(r.formatKey("payments", login))
		tx.HGet(r.formatKey("shares", "roundCurrent"), login)
		return nil
	})

	if err != nil && err != redis.Nil {
		return nil, err
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
	}

	return stats, nil
}

// Try to convert all numeric strings to int64
func convertStringMap(m map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	var err error
	for k, v := range m {
		result[k], err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			//If its IP Address, Dont Add those value to the Map for Security
			result[k] = v
		}
	}
	return result
}

// WARNING: Must run it periodically to flush out of window hashrate entries
func (r *RedisClient) FlushStaleStats(window, largeWindow time.Duration) (int64, error) {
	now := util.MakeTimestamp() / 1000
	max := fmt.Sprint("(", now-int64(window/time.Second))
	total, err := r.client.ZRemRangeByScore(r.formatKey("hashrate"), "-inf", max).Result()
	if err != nil {
		return total, err
	}

	var c int64
	miners := make(map[string]struct{})
	max = fmt.Sprint("(", now-int64(largeWindow/time.Second))

	for {
		var keys []string
		var err error
		c, keys, err = r.client.Scan(c, r.formatKey("hashrate", "*"), 100).Result()
		if err != nil {
			return total, err
		}
		for _, row := range keys {
			login := strings.Split(row, ":")[2]
			if _, ok := miners[login]; !ok {
				n, err := r.client.ZRemRangeByScore(r.formatKey("hashrate", login), "-inf", max).Result()
				if err != nil {
					return total, err
				}
				miners[login] = struct{}{}
				total += n
			}
		}
		if c == 0 {
			break
		}
	}
	return total, nil
}

func (r *RedisClient) CollectStats(smallWindow time.Duration, maxBlocks, maxPayments int64) (map[string]interface{}, error) {
	window := int64(smallWindow / time.Second)
	stats := make(map[string]interface{})

	tx := r.client.Multi()
	defer tx.Close()

	now := util.MakeTimestamp() / 1000

	cmds, err := tx.Exec(func() error {
		tx.ZRemRangeByScore(r.formatKey("hashrate"), "-inf", fmt.Sprint("(", now-window))
		tx.ZRangeWithScores(r.formatKey("hashrate"), 0, -1)
		tx.HGetAllMap(r.formatKey("stats"))
		tx.ZRevRangeWithScores(r.formatKey("blocks", "candidates"), 0, -1)
		tx.ZRevRangeWithScores(r.formatKey("blocks", "immature"), 0, -1)
		tx.ZRevRangeWithScores(r.formatKey("blocks", "matured"), 0, maxBlocks-1)
		tx.ZCard(r.formatKey("blocks", "candidates"))
		tx.ZCard(r.formatKey("blocks", "immature"))
		tx.ZCard(r.formatKey("blocks", "matured"))
		tx.ZCard(r.formatKey("payments", "all"))
		tx.ZRevRangeWithScores(r.formatKey("payments", "all"), 0, maxPayments-1)
		tx.LLen(r.formatKey("lastshares"))
		tx.HGetAllMap(r.formatKey("exchange", r.CoinName))
		return nil
	})

	if err != nil {
		return nil, err
	}

	result, _ := cmds[2].(*redis.StringStringMapCmd).Result()
	result["nShares"] = strconv.FormatInt(cmds[11].(*redis.IntCmd).Val(), 10)
	stats["stats"] = convertStringMap(result)
	candidates := convertCandidateResults(cmds[3].(*redis.ZSliceCmd))
	stats["candidates"] = candidates
	stats["candidatesTotal"] = cmds[6].(*redis.IntCmd).Val()

	immature := convertBlockResults(cmds[4].(*redis.ZSliceCmd))
	stats["immature"] = immature
	stats["immatureTotal"] = cmds[7].(*redis.IntCmd).Val()

	matured := convertBlockResults(cmds[5].(*redis.ZSliceCmd))
	stats["matured"] = matured
	stats["maturedTotal"] = cmds[8].(*redis.IntCmd).Val()

	payments := convertPaymentsResults(cmds[10].(*redis.ZSliceCmd))
	stats["payments"] = payments
	stats["paymentsTotal"] = cmds[9].(*redis.IntCmd).Val()

	totalHashrate, miners := convertMinersStats(window, cmds[1].(*redis.ZSliceCmd))
	stats["miners"] = miners
	stats["minersTotal"] = len(miners)
	stats["hashrate"] = totalHashrate

	exchangedata, _ := cmds[12].(*redis.StringStringMapCmd).Result()
	stats["exchangedata"] = exchangedata

	return stats, nil
}

func (r *RedisClient) CollectWorkersStats(sWindow, lWindow time.Duration, login string) (map[string]interface{}, error) {
	smallWindow := int64(sWindow / time.Second)
	largeWindow := int64(lWindow / time.Second)
	stats := make(map[string]interface{})

	tx := r.client.Multi()
	defer tx.Close()

	now := util.MakeTimestamp() / 1000

	cmds, err := tx.Exec(func() error {
		tx.ZRemRangeByScore(r.formatKey("hashrate", login), "-inf", fmt.Sprint("(", now-largeWindow))
		tx.ZRangeWithScores(r.formatKey("hashrate", login), 0, -1)
		tx.HGet(r.formatKey("shares", "roundCurrent"), login)
		tx.ZRevRangeWithScores(r.formatKey("rewards", login), 0, 39)
		tx.ZRevRangeWithScores(r.formatKey("rewards", login), 0, -1)
		return nil
	})

	if err != nil {
		return nil, err
	}

	totalHashrate := int64(0)
	currentHashrate := int64(0)
	online := int64(0)
	offline := int64(0)
	workers := convertWorkersStats(smallWindow, cmds[1].(*redis.ZSliceCmd))

	for id, worker := range workers {
		timeOnline := now - worker.startedAt
		if timeOnline < 600 {
			timeOnline = 600
		}

		boundary := timeOnline
		if timeOnline >= smallWindow {
			boundary = smallWindow
		}
		worker.HR = worker.HR / boundary

		boundary = timeOnline
		if timeOnline >= largeWindow {
			boundary = largeWindow
		}
		worker.TotalHR = worker.TotalHR / boundary

		if worker.LastBeat < (now - smallWindow/2) {
			worker.Offline = true
			offline++
		} else {
			online++
		}

		currentHashrate += worker.HR
		totalHashrate += worker.TotalHR
		workers[id] = worker
	}

	shares := cmds[2].(*redis.StringCmd).Val()

	stats["roundShares"] = shares
	//stats["shares"] = myshares
	stats["workers"] = workers
	stats["workersTotal"] = len(workers)
	stats["workersOnline"] = online
	stats["workersOffline"] = offline
	stats["hashrate"] = totalHashrate
	stats["currentHashrate"] = currentHashrate

	stats["rewards"] = convertRewardResults(cmds[3].(*redis.ZSliceCmd)) // last 90
	rewards := convertRewardResults(cmds[4].(*redis.ZSliceCmd))         // all

	var dorew []*SumRewardData
	dorew = append(dorew, &SumRewardData{Name: "Last 60 minutes", Interval: 3600, Offset: 0})
	dorew = append(dorew, &SumRewardData{Name: "Last 12 hours", Interval: 3600 * 12, Offset: 0})
	dorew = append(dorew, &SumRewardData{Name: "Last 24 hours", Interval: 3600 * 24, Offset: 0})
	dorew = append(dorew, &SumRewardData{Name: "Last 7 days", Interval: 3600 * 24 * 7, Offset: 0})
	dorew = append(dorew, &SumRewardData{Name: "Last 30 days", Interval: 3600 * 24 * 30, Offset: 0})

	for _, reward := range rewards {

		for _, dore := range dorew {
			dore.Reward += 0
			if reward.Timestamp > now-dore.Interval {
				dore.Reward += reward.Reward
			}
		}
	}
	stats["sumrewards"] = dorew
	stats["24hreward"] = dorew[2].Reward
	return stats, nil
}

func (r *RedisClient) CollectLuckStats(windows []int) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	tx := r.client.Multi()
	defer tx.Close()

	max := int64(windows[len(windows)-1])

	cmds, err := tx.Exec(func() error {
		tx.ZRevRangeWithScores(r.formatKey("blocks", "immature"), 0, -1)
		tx.ZRevRangeWithScores(r.formatKey("blocks", "matured"), 0, max-1)
		return nil
	})
	if err != nil {
		return stats, err
	}
	blocks := convertBlockResults(cmds[0].(*redis.ZSliceCmd), cmds[1].(*redis.ZSliceCmd))

	calcLuck := func(max int) (int, float64, float64, float64) {
		var total int
		var sharesDiff, uncles, orphans float64
		for i, block := range blocks {
			if i > (max - 1) {
				break
			}
			if block.Uncle {
				uncles++
			}
			if block.Orphan {
				orphans++
			}
			sharesDiff += float64(block.TotalShares) / float64(block.Difficulty)
			total++
		}
		if total > 0 {
			sharesDiff /= float64(total)
			uncles /= float64(total)
			orphans /= float64(total)
		}
		return total, sharesDiff, uncles, orphans
	}
	for _, max := range windows {
		total, sharesDiff, uncleRate, orphanRate := calcLuck(max)
		row := map[string]float64{
			"luck": sharesDiff, "uncleRate": uncleRate, "orphanRate": orphanRate,
		}
		stats[strconv.Itoa(total)] = row
		if total < max {
			break
		}
	}
	return stats, nil
}

func convertCandidateResults(raw *redis.ZSliceCmd) []*BlockData {
	var result []*BlockData
	for _, v := range raw.Val() {
		// "nonce:powHash:mixDigest:timestamp:diff:totalShares"
		block := BlockData{}
		block.Height = int64(v.Score)
		block.RoundHeight = block.Height
		fields := strings.Split(v.Member.(string), ":")
		block.Nonce = fields[0]
		block.PowHash = fields[1]
		block.MixDigest = fields[2]
		block.Timestamp, _ = strconv.ParseInt(fields[3], 10, 64)
		block.Difficulty, _ = strconv.ParseInt(fields[4], 10, 64)
		block.TotalShares, _ = strconv.ParseInt(fields[5], 10, 64)
		block.candidateKey = v.Member.(string)
		result = append(result, &block)
	}
	return result
}

func convertRewardResults(rows ...*redis.ZSliceCmd) []*RewardData {
	var result []*RewardData
	for _, row := range rows {
		for _, v := range row.Val() {
			// "amount:percent:immature:block.Hash:block.height"
			reward := RewardData{}
			reward.Timestamp = int64(v.Score)
			fields := strings.Split(v.Member.(string), ":")
			//block.UncleHeight, _ = strconv.ParseInt(fields[0], 10, 64)
			reward.BlockHash = fields[3]
			reward.Reward, _ = strconv.ParseInt(fields[0], 10, 64)
			reward.Percent, _ = strconv.ParseFloat(fields[1], 64)
			reward.Immature, _ = strconv.ParseBool(fields[2])
			reward.Height, _ = strconv.ParseInt(fields[4], 10, 64)
			result = append(result, &reward)
		}
	}
	return result
}

func convertBlockResults(rows ...*redis.ZSliceCmd) []*BlockData {
	var result []*BlockData
	for _, row := range rows {
		for _, v := range row.Val() {
			// "uncleHeight:orphan:nonce:blockHash:timestamp:diff:totalShares:rewardInWei"
			block := BlockData{}
			block.Height = int64(v.Score)
			block.RoundHeight = block.Height
			fields := strings.Split(v.Member.(string), ":")
			block.UncleHeight, _ = strconv.ParseInt(fields[0], 10, 64)
			block.Uncle = block.UncleHeight > 0
			block.Orphan, _ = strconv.ParseBool(fields[1])
			block.Nonce = fields[2]
			block.Hash = fields[3]
			block.Timestamp, _ = strconv.ParseInt(fields[4], 10, 64)
			block.Difficulty, _ = strconv.ParseInt(fields[5], 10, 64)
			block.TotalShares, _ = strconv.ParseInt(fields[6], 10, 64)
			block.RewardString = fields[7]
			block.ImmatureReward = fields[7]
			block.immatureKey = v.Member.(string)
			result = append(result, &block)
		}
	}
	return result
}

// Build per login workers's total shares map {'rig-1': 12345, 'rig-2': 6789, ...}
// TS => diff, id, ms
func convertWorkersStats(window int64, raw *redis.ZSliceCmd) map[string]Worker {
	now := util.MakeTimestamp() / 1000
	workers := make(map[string]Worker)

	for _, v := range raw.Val() {
		parts := strings.Split(v.Member.(string), ":")
		share, _ := strconv.ParseInt(parts[0], 10, 64)
		id := parts[1]
		score := int64(v.Score)
		worker := workers[id]

		// Add for large window
		worker.TotalHR += share

		// Add for small window if matches
		if score >= now-window {
			worker.HR += share
		}

		if worker.LastBeat < score {
			worker.LastBeat = score
		}
		if worker.startedAt > score || worker.startedAt == 0 {
			worker.startedAt = score
		}
		workers[id] = worker
	}
	return workers
}

func convertMinersStats(window int64, raw *redis.ZSliceCmd) (int64, map[string]Miner) {
	now := util.MakeTimestamp() / 1000
	miners := make(map[string]Miner)
	totalHashrate := int64(0)

	for _, v := range raw.Val() {
		parts := strings.Split(v.Member.(string), ":")
		share, _ := strconv.ParseInt(parts[0], 10, 64)
		id := parts[1]
		score := int64(v.Score)
		miner := miners[id]
		miner.HR += share

		if miner.LastBeat < score {
			miner.LastBeat = score
		}
		if miner.startedAt > score || miner.startedAt == 0 {
			miner.startedAt = score
		}
		miners[id] = miner
	}

	for id, miner := range miners {
		timeOnline := now - miner.startedAt
		if timeOnline < 600 {
			timeOnline = 600
		}

		boundary := timeOnline
		if timeOnline >= window {
			boundary = window
		}
		miner.HR = miner.HR / boundary

		if miner.LastBeat < (now - window/2) {
			miner.Offline = true
		}
		totalHashrate += miner.HR
		miners[id] = miner
	}
	return totalHashrate, miners
}

func convertPaymentsResults(raw *redis.ZSliceCmd) []map[string]interface{} {
	var result []map[string]interface{}
	for _, v := range raw.Val() {
		tx := make(map[string]interface{})
		tx["timestamp"] = int64(v.Score)
		fields := strings.Split(v.Member.(string), ":")
		tx["tx"] = fields[0]
		// Individual or whole payments row
		if len(fields) < 4 {
			tx["amount"], _ = strconv.ParseInt(fields[1], 10, 64)
			tx["txcost"], _ = strconv.ParseInt(fields[2], 10, 64)
		} else {
			tx["address"] = fields[1]
			tx["amount"], _ = strconv.ParseInt(fields[2], 10, 64)
			tx["txcost"], _ = strconv.ParseInt(fields[3], 10, 64)
		}
		result = append(result, tx)
	}
	return result
}

func (r *RedisClient) StoreExchangeData(ExchangeData []map[string]string) {

	tx := r.client.Multi()
	defer tx.Close()

	for _, coindata := range ExchangeData {
		for key, value := range coindata {

			cmd := tx.HSet(r.formatKey("exchange", coindata["symbol"]), key, value)
			err := cmd.Err()
			if err != nil {
				log.Printf("Error while Storing %s : Key-%s , value-%s , Error : %v", coindata["symbol"], key, value, err)
			}

		}
	}
	log.Printf("Writing Exchange Data ")
	return
}

func (r *RedisClient) GetExchangeData(coinsymbol string) (map[string]string, error) {

	cmd := r.client.HGetAllMap(r.formatKey("exchange", coinsymbol))

	result, err := cmd.Result()

	if err != nil {
		return nil, err
	}

	return result, err
}
