import Ember from 'ember';
import config from '../config/environment';

export default Ember.Controller.extend({  
   get config() {
        return config.APP;
    },

});
