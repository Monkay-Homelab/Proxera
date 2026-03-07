/* Proxera Panel — metrics-specific utilities and constants */

import type { MetricsBucket } from '$lib/types';

/** HTML-escape a string for safe tooltip rendering */
export function esc(s: unknown): string {
	if (s == null) return '';
	return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

/** Chart series label map */
export const chartLabels: Record<string, string> = {
	request_count: 'Requests', bytes_sent: 'Bytes Out', bytes_received: 'Bytes In',
	status_2xx: '2xx', status_3xx: '3xx', status_4xx: '4xx', status_5xx: '5xx',
	latency_p50_ms: 'p50', latency_p95_ms: 'p95', latency_p99_ms: 'p99',
	avg_upstream_ms: 'Upstream', avg_request_size: 'Req Size', avg_response_size: 'Res Size',
	cache_hits: 'Hits', cache_misses: 'Misses', unique_ips: 'Unique IPs', connection_count: 'Connections'
};

/** Chart color palette */
export const C = {
	green: '#42c990', blue: '#6C8EEF', purple: '#a78bfa',
	orange: '#e8a840', red: '#ef6068', cyan: '#38bdf8', pink: '#ec6fa2'
};

/** Time range options */
export const ranges = [
	{ value: '1h', label: '1H' },
	{ value: '6h', label: '6H' },
	{ value: '12h', label: '12H' },
	{ value: '24h', label: '1D' },
	{ value: '7d', label: '7D' },
	{ value: '30d', label: '30D' },
	{ value: '90d', label: '90D' },
	{ value: 'all', label: 'ALL' }
];

/** Country centroids [lat, lng] for heat dot placement */
export const countryCentroids: Record<string, [number, number]> = {
	AD:[42.5,1.5],AF:[33,65],AL:[41,20],DZ:[28,3],AO:[-12.5,18.5],AR:[-34,-64],AM:[40,45],AU:[-25,134],AT:[47.5,14],
	AZ:[40.5,47.5],BD:[24,90],BY:[53,28],BE:[50.8,4.5],BJ:[9.5,2.25],BO:[-17,-65],BA:[44,18],BW:[-22,24],
	BR:[-10,-55],BG:[43,25],BF:[13,-1.5],BI:[-3.5,30],KH:[13,105],CM:[6,12.5],CA:[60,-96],CF:[7,21],
	TD:[15,19],CL:[-30,-71],CN:[35,105],CO:[4,-72],CD:[-3,24],CG:[-1,15.5],CR:[10,-84],CI:[8,-5.5],
	HR:[45.2,15.5],CU:[22,-80],CY:[35,33],CZ:[49.75,15.5],DK:[56,10],DJ:[11.5,43],DO:[19,-70.7],
	EC:[-2,-77.5],EG:[27,30],SV:[13.8,-88.9],GQ:[2,10],ER:[15.5,39],EE:[59,26],SZ:[-26.5,31.5],
	ET:[8,38],FI:[64,26],FR:[46,2],GA:[-1,11.7],GM:[13.5,-15.5],GE:[42,43.5],DE:[51,9],GH:[8,-2],
	GR:[39,22],GT:[15.5,-90.3],GN:[11,-12],GW:[12,-15],GY:[5,-59],HT:[19,-72.3],HN:[15,-86.5],
	HU:[47,20],IS:[65,-18],IN:[22,79],ID:[-2.5,118],IR:[32,53],IQ:[33,44],IE:[53,-8],IL:[31.5,35],
	IT:[42.8,12.8],JM:[18.2,-77.5],JP:[36,138],JO:[31,36.5],KZ:[48,68],KE:[-1,38],KP:[40,127],
	KR:[36,128],KW:[29.5,47.8],KG:[41,75],LA:[18,105],LV:[57,25],LB:[33.8,35.8],LS:[-29.5,28.5],
	LR:[6.5,-9.5],LY:[27,17],LT:[56,24],LU:[49.75,6.2],MG:[-20,47],MW:[-13.5,34],MY:[2.5,112.5],
	ML:[17,-4],MR:[20,-12],MX:[23,-102],MD:[47,29],MN:[46,105],ME:[42.5,19.3],MA:[32,-6],MZ:[-18.3,35],
	MM:[22,96],NA:[-22,17],NP:[28,84],NL:[52.5,5.75],NZ:[-42,174],NI:[13,-85.2],NE:[16,8],NG:[10,8],
	MK:[41.5,21.4],NO:[62,10],OM:[21,57],PK:[30,70],PA:[9,-80],PG:[-6,147],PY:[-23,-58],PE:[-10,-76],
	PH:[13,122],PL:[52,20],PT:[39.5,-8],QA:[25.5,51.3],RO:[46,25],RU:[60,100],RW:[-2,29.9],SA:[24,45],
	SN:[14.5,-14.5],RS:[44,21],SL:[8.5,-11.8],SG:[1.4,103.8],SK:[48.7,19.7],SI:[46.1,15],SO:[6,46],
	ZA:[-30,26],SS:[7,30],ES:[40,-4],LK:[7.5,81],SD:[16,30],SR:[4,-56],SE:[62,15],CH:[47,8.2],
	SY:[35,38],TW:[23.5,121],TJ:[39,71],TZ:[-6.5,35],TH:[15,101],TG:[8.6,1.2],TT:[10.5,-61.3],
	TN:[34,9],TR:[39,35],TM:[40,60],UG:[1.5,32.5],UA:[49,32],AE:[24,54],GB:[54,-2],US:[39,-98],
	UY:[-33,-56],UZ:[41.5,65],VE:[8,-66],VN:[16,108],YE:[15.5,48],ZM:[-15,28],ZW:[-20,30]
};

/*
 * Metrics-specific formatters — intentionally different from $lib/utils versions.
 * These use higher precision (.toFixed(2) for seconds, uppercase "K") for chart
 * tooltips and axis labels where visual consistency matters.
 */

export function formatBytes(bytes: number): string {
	if (bytes === 0) return '0 B';
	const k = 1024, sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

export function formatNumber(n: number): string {
	if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
	if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
	return n.toString();
}

export function formatMs(ms: number): string {
	return ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms.toFixed(1) + 'ms';
}

export function formatTime(ts: string, selectedRange: string): string {
	const d = new Date(ts);
	if (['7d','30d','90d','all'].includes(selectedRange)) {
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
	}
	return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

export function formatFullTime(ts: string): string {
	const d = new Date(ts);
	return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

export function formatBlockedTime(ts: string): string {
	if (!ts) return '-';
	const d = new Date(ts);
	return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

/* ── Bucket aggregation ── */

export function aggregateBuckets(buckets: MetricsBucket[]): MetricsBucket[] {
	const map: Record<string, MetricsBucket & Record<string, number>> = {};
	for (const b of buckets) {
		const key = b.time;
		if (!map[key]) {
			map[key] = { time: b.time, request_count:0, bytes_sent:0, bytes_received:0, status_2xx:0, status_3xx:0, status_4xx:0, status_5xx:0, _latency_weight:0, _p50_weight:0, _p95_weight:0, _p99_weight:0, _upstream_weight:0, _reqsize_weight:0, _ressize_weight:0, avg_latency_ms:0, latency_p50_ms:0, latency_p95_ms:0, latency_p99_ms:0, avg_upstream_ms:0, avg_request_size:0, avg_response_size:0, cache_hits:0, cache_misses:0, unique_ips:0, connection_count:0 };
		}
		const a = map[key], rc = b.request_count||0;
		a.request_count+=rc; a.bytes_sent+=b.bytes_sent||0; a.bytes_received+=b.bytes_received||0;
		a.status_2xx+=b.status_2xx||0; a.status_3xx+=b.status_3xx||0; a.status_4xx+=b.status_4xx||0; a.status_5xx+=b.status_5xx||0;
		a._latency_weight+=(b.avg_latency_ms||0)*rc; a._p50_weight+=(b.latency_p50_ms||0)*rc; a._p95_weight+=(b.latency_p95_ms||0)*rc; a._p99_weight+=(b.latency_p99_ms||0)*rc;
		a._upstream_weight+=(b.avg_upstream_ms||0)*rc; a._reqsize_weight+=(b.avg_request_size||0)*rc; a._ressize_weight+=(b.avg_response_size||0)*rc;
		a.cache_hits+=b.cache_hits||0; a.cache_misses+=b.cache_misses||0; a.unique_ips+=b.unique_ips||0; a.connection_count+=b.connection_count||0;
	}
	return Object.values(map).map(a => {
		if (a.request_count>0) { a.avg_latency_ms=a._latency_weight/a.request_count; a.latency_p50_ms=a._p50_weight/a.request_count; a.latency_p95_ms=a._p95_weight/a.request_count; a.latency_p99_ms=a._p99_weight/a.request_count; a.avg_upstream_ms=a._upstream_weight/a.request_count; a.avg_request_size=a._reqsize_weight/a.request_count; a.avg_response_size=a._ressize_weight/a.request_count; }
		return a;
	}).sort((a,b) => new Date(a.time).getTime()-new Date(b.time).getTime());
}
