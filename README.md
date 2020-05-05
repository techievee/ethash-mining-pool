# Open-ethereum-pool
Highly Efficient mining pool designed for ETC coin


![alt text](https://raw.githubusercontent.com/techievee/ethash-mining-pool/master/images/FrontEnd.png)

### Features

**This pool is reegineered from sammy007 open-ethereum-pool for efficiency and for better payment algorithm. This software is functional and tested and implemented in big Mining farms!

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

### Watch setup guide video on YouTube channel

https://www.youtube.com/watch?v=NOjPFZk4sp0


### Building on Linux

Dependencies:

    apt install software-properties-common unzip nginx tor pcscd pcsc-tools git curl htop mc unrar screen

**I highly recommend to use Ubuntu 18.04.4 LTS.**


Geth setup:

    sudo su
    mkdir /pool && cd /pool
    wget https://github.com/multi-geth/multi-geth/releases/download/v1.9.7/multi-geth-linux.zip
    unzip multi-geth-linux.zip
    ln -s /pool/geth /usr/bin/geth
    chmod ugo+x /usr/bin/geth

    geth --classic account new
    geth --classic account list

example output:

Public address of the key: 0xa9c96bff41FFAFb6bE8c2990c0CF4F3b220aaEaa
Path of the secret key file: /root/.ethereum/classic/keystore/UTC--2020-03-11T22-03-07.512690005Z--a9c96bff41ffafb6be8c2990c0cf4f3b220aaeaa

Create geth start script:

    nano /pool/start_geth.sh

    #!/bin/bash
    screen -S server geth --classic --rpc --maxpeers 75 --syncmode "fast" --rpcapi "eth,net,web3,personal" --etherbase "YOUR_WALLET" --cache=12288 --mine --unlock "YOUR_WALLET" --allow-insecure-unlock --password /pool/pwd

Enter wallet password to the file: 
    
    nano /pool/pwd

    chmod +x /pool/start_geth.sh

Start blockchain sync:

    ./start_geth.sh

In new console window run this commands:

    sudo su
    cd /pool
    git config --global http.https://gopkg.in.followRedirects true
    git clone https://github.com/notminerproduction/open-ethereum-pool.git
    cd open-ethereum-pool
    chmod +x ./build/env.sh
    add-apt-repository ppa:longsleep/golang-backports
    apt update
    apt install golang-go
    make
    add-apt-repository ppa:chris-lea/redis-server
    apt update
    apt install redis-server
    systemctl enable redis-server.service && systemctl stop redis-server.service && systemctl start redis-server.service

    curl -sL https://deb.nodesource.com/setup_13.x | bash -
    curl -sL https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add -
    echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list

    apt update
    apt install nodejs yarn


### Building Frontend

    cd www/
    
Change 192.168.0.200 to your IP or DNS in file environment.js:
    
    nano config/environment.js

Then run this commands:

    sudo npm install -g ember-cli@2.9.1
    sudo npm install -g bower
    sudo npm install fsevents@latest -f --save-optional
    sudo npm install intl-format-cache@4.2.22
    sudo npm install format-number
    sudo npm install ember-cli-accounting
    sudo npm install core-js@3.6.4
    sudo npm install jquery@3.4.0
    sudo npm install @babel/core@^7.0.0-beta.42
    sudo npm install babel-plugin-debug-macros@0.2.0
    sudo npm install ember-intl@4.3.0
    sudo npm install minimatch@3.0.2
    sudo npm install ember-cli-babel@7.18.0
    sudo npm install ember-resolver@7.0.0
    sudo npm install
    sudo npm audit fix
    sudo npm audit fix --force
    bower install --allow-root
    wget https://files.gitter.im/sammy007/open-ethereum-pool/IBJl/intl-format-cache.rar
    unrar x intl-format-cache.rar node_modules/intl-format-cache/ -Y
    chmod +x build.sh
    ./build.sh
    cd ../
    cp misc/nginx-default.conf /etc/nginx/sites-available/default
    systemctl enable nginx.service && systemctl stop nginx.service && systemctl start nginx.service
    screen -S pool ./build/bin/ethash-mining-pool config_api.json
    


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
        "threads": 2,
        "coin": "etc-pplns",
        "name": "main",
        "pplns": 15000,
        "coin-name":"ETC",

        "proxy": {
                "enabled": true,
                "listen": "0.0.0.0:40002",
                "limitHeadersSize": 1024,
                "limitBodySize": 256,
                "behindReverseProxy": false,
                "blockRefreshInterval": "120ms",
                "stateUpdateInterval": "3s",
                "difficulty": 4000000000,
		"DifficultyNiceHash": 4,
                "hashrateExpiration": "3h",

                "healthCheck": true,
                "maxFails": 100,

                "stratum": {
                        "enabled": true,
                        "listen": "0.0.0.0:9009",
                        "timeout": "120s",
                        "maxConn": 8192
                },
                "stratum_nice_hash": {
                        "enabled": true,
                        "listen": "0.0.0.0:40004",
                        "timeout": "120s",
                        "maxConn": 8192
                },

                "policy": {
                        "workers": 8,
                        "resetInterval": "60m",
                        "refreshInterval": "1m",

                        "banning": {
                                "enabled": true,
                                "ipset": "blacklist",
                                "timeout": 300,
                                "invalidPercent": 30,
                                "checkThreshold": 30,
                                "malformedLimit": 50
                        },
                        "limits": {
                                "enabled": true,
                                "limit": 30,
                                "grace": "5m",
                                "limitJump": 10
                        }
                }
        },

        "api": {
                "enabled": true,
                "purgeOnly": false,
                "purgeInterval": "10m",
                "listen": "0.0.0.0:9091",
                "statsCollectInterval": "5s",
                "hashrateWindow": "30m",
                "hashrateLargeWindow": "3h",
                "luckWindow": [64, 128, 256],
                "payments": 30,
                "blocks": 50
        },

        "upstreamCheckInterval": "5s",
        "upstream": [
                {
                        "name": "main",
                        "url": "http://127.0.0.1:8545",
                        "timeout": "10s"
                },
                {
                        "name": "backup",
                        "url": "http://127.0.0.2:8545",
                        "timeout": "10s"
                }
        ],

        "redis": {
                "endpoint": "127.0.0.1:6379",
                "poolSize": 300,
                "database": 0,
                "password": ""
        },

        "unlocker": {
                "enabled": true,
                "poolFee": 0.5,
                "poolFeeAddress": "0xa9c96bff41FFAFb6bE8c2990c0CF4F3b220aaEaa",
                "donate": false,
                "depth": 120,
                "immatureDepth": 20,
                "keepTxFees": false,
                "interval": "15m",
                "daemon": "http://127.0.0.1:8545",
                "timeout": "10s"
        },

        "payouts": {
                "enabled": true,
                "requirePeers": 5,
                "interval": "120m",
                "daemon": "http://127.0.0.1:8545",
                "timeout": "10s",
                "address": "0xa9c96bff41FFAFb6bE8c2990c0CF4F3b220aaEaa",
                "gas": "21000",
                "gasPrice": "20000000000",
                "autoGas": true,
                "keepNwFees": true,
                "nwTxGas": "21000",
                "nwTxGasPrice": "20000000000",
                "threshold": 100000000,
                "bgsave": false
        },

        "exchange": {
                "enabled": false,
                "url": "https://api.coinmarketcap.com/v1/ticker/ethereum-classic/?convert=USD",
                "timeout": "50s",
                "refreshInterval": "1800s"
        },

        "newrelicEnabled": false,
        "newrelicName": "",
        "newrelicKey": "",
        "newrelicVerbose": false
}



```






