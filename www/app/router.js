import Ember from 'ember';
import config from './config/environment';

var Router = Ember.Router.extend({
  location: config.locationType
});

Router.map(function() {
  this.route('account', { path: '/account/:login' }, function() {
    this.route('payouts');
    this.route('rewards');
    this.route('settings');
  });
  this.route('not-found');

  this.route('blocks', function() {
    this.route('mature');
    this.route('immature');
    this.route('pending');
    
  });
  

  this.route('help');
  this.route('payments');
  this.route('miners');
  this.route('exchange');
  this.route('about');
   this.route('helpdesk', { path: 'http://helpdesk.miningpoolindia.com' });
});

export default Router;
