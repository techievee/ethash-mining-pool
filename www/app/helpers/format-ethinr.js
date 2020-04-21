import Ember from 'ember';

export function formatEthInr(params) {
	let value = params[0];       
        let ethinr = params[1];
        let valueeth = value * 0.000000001;
        let inr = valueeth * ethinr;
        
        if (isNaN(inr)) { 
            return ''; 
        }
        
	return "₹" + inr.toFixed(2);
}

export default Ember.Helper.helper(formatEthInr);

