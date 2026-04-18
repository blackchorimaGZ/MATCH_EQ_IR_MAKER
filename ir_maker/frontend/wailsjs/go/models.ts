export namespace dsp {
	
	export class AnalysisResult {
	    ref_spectrum: number[];
	    tgt_spectrum: number[];
	    ir_spectrum: number[];
	
	    static createFrom(source: any = {}) {
	        return new AnalysisResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref_spectrum = source["ref_spectrum"];
	        this.tgt_spectrum = source["tgt_spectrum"];
	        this.ir_spectrum = source["ir_spectrum"];
	    }
	}

}

