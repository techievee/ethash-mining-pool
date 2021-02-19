import Ember from 'ember';
import config from '../config/environment';

export default Ember.Controller.extend({
  applicationController: Ember.inject.controller('application'),
  stats: Ember.computed.reads('applicationController.model.stats'),
  config: Ember.computed.reads('applicationController.config'),

  
  roundPercent: Ember.computed('stats', 'model', {
    get() {
      var percent = this.get('model.roundShares') / this.get('stats.nShares');
      if (!percent) {
        return 0;
      }
      if(percent>100){
          return 100;
      }
          
      return percent;
    }
  }),
  
  
  payoutthreshold: Ember.computed('model', {
    get() {
      var defaultThreshold = config.APP.PayoutThreshold;
      var threshold = this.getWithDefault('model.stats.payoutthreshold',defaultThreshold);
        return threshold;
    }
  }),
  
  
  paymentPercent: Ember.computed('model',{
    get() {
      var defaultThreshold = config.APP.PayoutThreshold;
      defaultThreshold =defaultThreshold * 1000000000;
      var threshold = this.getWithDefault('model.stats.payoutthreshold',defaultThreshold);
      var balance = this.get('model.stats.balance');
            
      var percent = 100;
      threshold = threshold * 0.000000001;
      balance = balance * 0.000000001;
      
      
      
      percent = (balance * 100) / threshold ;
      
      if (!percent) {
        return 0;
      }
      else if(percent>100){
          return 100;
      }
      else{
           return percent.toFixed(2);
      }
          
      return percent.toFixed(2);
    }
  })
});
