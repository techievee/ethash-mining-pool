# Ethash-mining-pool
Highly Efficient mining pool designed for Ethash based coins
[![Build Status](https://travis-ci.org/techievee/ethash-mining-pool.svg?branch=V2.0_Eth)](https://travis-ci.org/techievee/ethash-mining-pool) 


![alt text](https://raw.githubusercontent.com/techievee/ethash-mining-pool/master/images/FrontEnd.png)

### Features

**This pool is reegineered from sammy007 open-ethereum-pool for efficiency and for better payment algorithm. This software is functional and tested and implemented in big Mining farms. Testing and bug submissions are still welcome!

*	Support for HTTP, SSL, Stratum, Stratum+SSL mining
*	Detailed block stats with luck percentage and full reward
*	Failover node instances: high availability built in (Any number of full nodes can be added to the configurations)
*	Modern beautiful Ember.js frontend for Individual coin statistics vs consolidated coin statistics
*	Separate stats for workers: can highlight timed-out workers so miners can perform maintenance of rigs
*	JSON-API for statistics, (Looking for contributor to build app for the pool)
*	Dynamic PPLNS block reward (*New)

### How it is different from sammy007 version

*	Reengineered most of the code for efficiency and Scaling
*	New Dynamic PPLNS Reward System
*	Integration with the Exchange to get real-time conversion between crypto and fiat.
*	The Network fees can be configured to be withheld for every transfers. 
*	The gas can be set as Auto and made to deduct automatically or can be fixed by the pool operator
*	Many parameters configurable using config file
*	Nicehash support *Not tested
*	SSL Support built-in
*	Very attractive frontend with more detailed statistics


### Branches 
**Please clone the project using the branches as  per the coin

* V2.0_Eth – Ethereum pool for go version <=1.10
*	V2.0_Eth_11 -– Ethereum pool for go version >=1.11
*	V2.0_Etc – Ethereum Classicpool for go version <=1.10
*	V2.0_Exp – Expanse pool for go version <=1.10
*	V2.0_Ubiq – Ubiq pool for go version <=1.10
*	V2.0_Pirl – Pirl pool for go version <=1.10
*	V2.0_Dbix – Dubai coin pool for go version <=1.10
*	V2.0_Soil – Soil Coin pool for go version <=1.10
*	V2.0_Mc – Music Coin pool for go version <=1.10
*	V2.0_Nuko – Nuko coin - * Not implemented

### Architecture

![Architecture](https://raw.githubusercontent.com/techievee/ethash-mining-pool/master/images/Architecture.PNG)


### Building on Linux

Dependencies:

  * go >= 1.9 but <1.10 ( For 1.11 Refer branches)
  * geth or parity
  * redis-server >= 2.8.0
  * nodejs >= 4 LTS
  * nginx

**I highly recommend to use Ubuntu 16.04 LTS.**

First install  [go-ethereum](https://github.com/ethereum/go-ethereum/wiki/Installation-Instructions-for-Ubuntu).

Clone & compile:

    git config --global http.https://gopkg.in.followRedirects true
    git clone https://github.com/techievee/ethash-mining-pool.git
    <Change the branch>
    git pull
    cd open-ethereum-pool
    make

Install redis-server.


### Building Frontend

There are 2 front end application. 
* One for showing the consolidated view of different pool statistics
* Another for single coin frontend.

You may choose your desired one
To show the statistics for one coin (Front end api) : https://github.com/techievee/statistics_api 
To show the consolidated statistics for all the coins that were implemented : https://github.com/techievee/generalapi

The frontend is a single-page Ember.js application that polls the pool API to render miner stats.
Please refer the frontend application Readme.md for more information. Those frontend are not compliant to sammy007 versions

### DPPLNS ALGORITHM (NEW) 

The algorithm explanation is as follows

* CALCULATING THE LAST N VALUE
```javascript
Check whether blockchainnode present
If present
	Calulcate the current network difficulty
	Calulate the network difficulty *2
	Calculate the N value by (2* Network difficulty)/ Share difficulty
 	Set the last N value in the statistics hash key
Else
	Fix the predetermined standard ‘N’ value from the block chain	
```
Space : O(1)	
RunTime : O(1)

* ADJUSTING THE ‘N’ SHARES
```javascript
Get the current last N value from the lastNshares Hash
Get the current count of lastNshares list
If lastNShare< count
	Loop(count - lastNshare )
		Pop lastNShare
		Decrement the miners share count from the lastN value
    		Decrement the total shares count
       end
```

Space : O(1)	
RunTime : O(N)

* NEW SHARE SUBMISSION
```javascript
Push the value of miners address into the last shares list
Increment the current round shares by 1
Increment the round shares value by 1
Increment the miners shares by 1
Check if new block found
If found
	Run the New block function
Else
Adjust the value of ‘N’ shares
```

Space : O(1)	
RunTime : O(1)

* REWARD CALUCLATION USING DPPLNS
```javascript
Loop(lastNshare )
		Get the miners address from list
Increment the local miners current round share 
       End
Loop(miners of current round)
		Percentage= minersshare / current round share
End
```

Space : O(N)	
RunTime : O(N)


### Configuration Documentation- Applies to all brnanches

Configuration is actually simple, just read it twice and think twice before changing defaults.

```javascript
{
	//Number of Go process to be used for running this software
	"threads": 2,
	// Used for prepend Redis instance
	"coin": "ethereum-pplns",

	"name": "main",

	//Initial N value of the reward, before the round starts
	"pplns": 1000000,

	//Used for fetching the coin value from the Exchange
	"coin-name":"ETH",

	"proxy": {
	    //Proxy Enabled or not
		"enabled": false,

		//Which HTTP port the mining instance if running
		"listen": "0.0.0.0:8888",
		"limitHeadersSize": 1024,
		"limitBodySize": 256,

		//If you are behind reverse proxy then enable, Doesnt work with Cloudflare,Works with Nginx
		"behindReverseProxy": false,

		//Can reduce upto 50ms, if you have good configuration server
		"blockRefreshInterval": "120ms",
		"stateUpdateInterval": "3s",

		//Can be changed according your pool setup, if you have powerful rigs then customize it
		"difficulty": 4000000000,

		//The hash rate of miner detoriate slowly in 3 hours
		"hashrateExpiration": "3h",

		//Checks the Health of the NOde server
		"healthCheck": true,
		"maxFails": 100,

		//Setting to enable TCP Startum mining, use this always to enable HTTP overhead
		"stratum": {
			"enabled": false,
			"listen": "0.0.0.0:8008",
			"timeout": "120s",
			"maxConn": 8192
		},

		//Nice Hash settings, not tested
		"stratum_nice_hash": {
			"enabled": false,
			"listen": "0.0.0.0:8089",
			"timeout": "120s",
			"maxConn": 8192
		},

		//Policy for banning spammers, disable for local mining
		"policy": {
			"workers": 8,
			"resetInterval": "60m",
			"refreshInterval": "1m",

			"banning": {
				"enabled": true,
				//Create a new firewall ipset with the name to make use of it, It bans at OS level
				"ipset": "blacklist",
				"timeout": 300,
				"invalidPercent": 30,
				"checkThreshold": 30,
				"malformedLimit": 50
			},

			//Limits are used to give grace to new miners for illegal shares.
			"limits": {
				"enabled": true,
				"limit": 30,
				"grace": "5m",
				"limitJump": 10
			}
		}
	},

    //These settings are enabled, if you want to enable the API for serving the front end.
    //You can run the API in seperate node so that the mining are not affected due to frontend load

	"api": {
		"enabled": false,
		"purgeOnly": false,
		"purgeInterval": "10m",
		"listen": "0.0.0.0:8081",

		//Intervel between the statistics collection
		"statsCollectInterval": "5s",
		"hashrateWindow": "30m",
		"hashrateLargeWindow": "3h",

		//Luck calculation for the window for the last 3 sets of blocks
		"luckWindow": [64, 128, 256],

		//Total number of payments to be shown in payment history
		"payments": 30,

		//Total number of blocks to be shown in block history
		"blocks": 50
	},

	"upstreamCheckInterval": "5s",

	//Full node of the coin instance
	"upstream": [
		//Node 1
		{
			"name": "mumbai",
			"url": "http://10.4.1.10:8000",
			"timeout": "10s"
		},

		//Node 2
		{
			"name": "backup",
			"url": "http://10.4.1.11:8545",
			"timeout": "10s"
		}
	],

    //Redis instances
	"redis": {
		"endpoint": "10.4.10.100:6379",
		//Increase the pool size if the load is more
		"poolSize": 10,
		"database": 8,
		"password": "9B3D8aD6705d246C82dBaA0DC10D2C7Ce47115Af"
	},


//Run the unlocker separately, as this is the critical node from where the rewards are released
	"unlocker": {
		"enabled": true,

		//Percentage to be retained as Pool fees
		"poolFee": 0.99,

		//Where the pool fee need to be transferred, Leave it blank to retain it in pool mining address
		"poolFeeAddress": "0x9dbe1caae874baae91022d170fba246100c73286",

		"donate": false,

		//Total number of confirmation to be wait before calculating the Rewards share
		"depth": 120,

		//Yellow signal after confirmation- Immature blocks
		"immatureDepth": 20,

		//Do you need to retain the network fees?
		"keepTxFees": false,
		"interval": "15m",
		"daemon": "http://127.0.0.1:8880",
		"timeout": "10s"
	},

//Run the Payout separately and run when its required, as this is the critical node from where the coin is released
//The payout address need to be unlocked while running this instance
	"payouts": {
		"enabled": false,

		//Number of peers need to be connected before executing the payout
		"requirePeers": 5,
		"interval": "120m",

		//Node where the Payout instance is running with payout address unlocked ( BE CAREFUL)
		"daemon": "http://127.0.0.1:8880",
		"timeout": "10s",

		//Address from the Rewards are paid
		"address": "0xff2a46e45df2b49f388f523b43153c38079237d9",

		//Predefined Gas and GasPrice, if the autogas is not enabled.
		"gas": "21000",
		"gasPrice": "20000000000",
		"autoGas": true,

		//Do you want to retain the Network Fees
		"keepNwFees": true,
		"nwTxGas": "21000",
		"nwTxGasPrice": "20000000000",
		"threshold": 10000000,

		//Run the bgsave after executing the payment, to save the redis state to disk
		//bgsave need to be configured in Redis instance
		"bgsave": false
	},

//Run this along with the API, to fetch the Converion rates from the coin market
//Please refer their API for any updates
	"exchange": {
		"enabled": false,
		"url": "https://api.coinmarketcap.com/v1/ticker/?convert=INR",
		"timeout": "50s",
		"refreshInterval": "1800s"
	},

//If you want to use the NewRelic to monitor the server status and the internal status,
//Register account with NewRelic and update the parameters here
	"newrelicEnabled": false,
	"newrelicName": "PROXY01_ETH_MUMBAI",
	"newrelicKey": "b30cd005408cbe2bcef6e081dc0a7ea2fe6c6a18",
	"newrelicVerbose": false
}


```

### Sample VM Configurations

![Configuration](https://raw.githubusercontent.com/techievee/ethash-mining-pool/master/images/Configurations.PNG)






