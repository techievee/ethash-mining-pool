/* jshint node: true */

module.exports = function (environment) {
    var ENV = {
        modulePrefix: 'open-ethereum-pool',
        environment: environment,
        rootURL: '/',
        locationType: 'hash',
        EmberENV: {
            FEATURES: {
                // Here you can enable experimental features on an ember canary build
                // e.g. 'with-controller': true
            }
        },

        APP: {
            // API host and port
            ApiUrl: '//192.168.0.200/',

            // HTTP mining endpoint
            HttpHost: '192.168.0.200',
            HttpPort: 40002,

            // Stratum mining endpoint
            StratumHost: '192.168.0.200',
            StratumPort: 9009,

            // Stratum SSL mining endpoint
            StratumHost1: '192.168.0.200',
            StratumPort1: 8008,

            NicehashHost: '',
            NicehashPost: 40004,

            // Fee and payout details
            PoolFee: '0.5%',
            PayoutThreshold: '0.5',
            ShareDifficulty: '4000000000',

            //Current and Localization
            Currency: 'USD',
            CoinName: 'Ethereum Classic',
            CoinShortName: 'ETC',
            PaymentText: 'every hour',
            SupportMail: '',
            SupportHelpdesk: '',
            WebsiteName: '192.168.0.200',

            //Coin Bases Settings
            ChainAddress : 'https://etcblockexplorer.com/addr/',
	        TransactionAddress : 'https://minergate.com/blockchain/etc/transaction/',
            UncleAddress : 'https://minergate.com/blockchain/etc/block/',
            BlockAddress : 'https://minergate.com/blockchain/etc/block/',

            //Twitter Parameter
            TwitterURL: '',
            TwitterHash: '',


            // For network hashrate (change for your favourite fork)
            BlockTime: 14
        }
    };

    if (environment === 'development') {
        /* Override ApiUrl just for development, while you are customizing
         frontend markup and css theme on your workstation.
         */
        ENV.APP.ApiUrl = 'https://etc.daggerpool.com/'
        // ENV.APP.LOG_RESOLVER = true;
        // ENV.APP.LOG_ACTIVE_GENERATION = true;
        // ENV.APP.LOG_TRANSITIONS = true;
        // ENV.APP.LOG_TRANSITIONS_INTERNAL = true;
        // ENV.APP.LOG_VIEW_LOOKUPS = true;
    }

    if (environment === 'test') {
        // Testem prefers this...
        ENV.locationType = 'none';

        // keep test console output quieter
        ENV.APP.LOG_ACTIVE_GENERATION = false;
        ENV.APP.LOG_VIEW_LOOKUPS = false;

        ENV.APP.rootElement = '#ember-testing';
    }

    if (environment === 'production') {
        ENV.baseURL = '/ember-cli-twitter-feed/';
    }

    ENV.contentSecurityPolicy = {
        'script-src': [
            "'self'",
            'https://syndication.twitter.com',
			            'https://cdn.syndication.twimg.com',
            "'sha256-XnNQECY9o-nIv2Qgcd1A39YarwxTm10rhdzegH_JBxY='"],
        'style-src': [
            "'self'",
            'http://platform.twitter.com',
            "'sha256-zCvYlDs6LsUp0EqrJFjIGUiM_AG2fGlkNrzJ2YiBTG0='"],
        'img-src': [
            "'self'",
            'data:',
            'https://pbs.twimg.com',
            'https://syndication.twitter.com',
            'http://platform.twitter.com']
    };
    return ENV;
};
