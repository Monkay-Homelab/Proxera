<script lang="ts">
	import { formatNumber, esc } from '$lib/metricsUtils';
	import type { Visitor } from '$lib/types';

	interface TooltipState {
		visible: boolean;
		x?: number;
		y?: number;
		html?: string;
	}

	interface GeoFeature {
		id: number | string;
		geometry: {
			type: string;
			coordinates: number[][][] | number[][][][];
		};
	}

	interface WorldGeoData {
		features: GeoFeature[];
	}

	interface CountryData {
		count: number;
		country: string;
		code: string;
	}

	interface HitArea {
		alpha2: string;
		coords: number[][][] | number[][][][];
		type: string;
		data: CountryData | null;
	}

	interface Props {
		visitors?: Visitor[];
		worldGeo?: WorldGeoData | null;
		onTooltip?: (state: TooltipState) => void;
	}

	let { visitors = [], worldGeo = null, onTooltip = () => {} }: Props = $props();

	let canvas: HTMLCanvasElement | null = null;
	let countryHitAreas: HitArea[] = [];
	let resizeObserver: ResizeObserver | null = null;

	/** ISO 3166-1 numeric → alpha-2 mapping for country identification */
	const numToAlpha2: Record<string, string> = {
		'4':'AF','8':'AL','12':'DZ','24':'AO','32':'AR','36':'AU','40':'AT','31':'AZ',
		'50':'BD','56':'BE','204':'BJ','68':'BO','70':'BA','72':'BW','76':'BR','100':'BG',
		'854':'BF','108':'BI','116':'KH','120':'CM','124':'CA','140':'CF','148':'TD','152':'CL',
		'156':'CN','170':'CO','180':'CD','178':'CG','188':'CR','384':'CI','191':'HR','192':'CU',
		'196':'CY','203':'CZ','208':'DK','262':'DJ','214':'DO','218':'EC','818':'EG','222':'SV',
		'226':'GQ','232':'ER','233':'EE','748':'SZ','231':'ET','246':'FI','250':'FR','266':'GA',
		'270':'GM','268':'GE','276':'DE','288':'GH','300':'GR','320':'GT','324':'GN','624':'GW',
		'328':'GY','332':'HT','340':'HN','348':'HU','352':'IS','356':'IN','360':'ID','364':'IR',
		'368':'IQ','372':'IE','376':'IL','380':'IT','388':'JM','392':'JP','400':'JO','398':'KZ',
		'404':'KE','408':'KP','410':'KR','414':'KW','417':'KG','418':'LA','428':'LV','422':'LB',
		'426':'LS','430':'LR','434':'LY','440':'LT','442':'LU','450':'MG','454':'MW','458':'MY',
		'466':'ML','478':'MR','484':'MX','498':'MD','496':'MN','499':'ME','504':'MA','508':'MZ',
		'104':'MM','516':'NA','524':'NP','528':'NL','554':'NZ','558':'NI','562':'NE','566':'NG',
		'807':'MK','578':'NO','512':'OM','586':'PK','591':'PA','598':'PG','600':'PY','604':'PE',
		'608':'PH','616':'PL','620':'PT','634':'QA','642':'RO','643':'RU','646':'RW','682':'SA',
		'686':'SN','688':'RS','694':'SL','702':'SG','703':'SK','705':'SI','706':'SO','710':'ZA',
		'728':'SS','724':'ES','144':'LK','729':'SD','740':'SR','752':'SE','756':'CH','760':'SY',
		'158':'TW','762':'TJ','834':'TZ','764':'TH','768':'TG','780':'TT','788':'TN','792':'TR',
		'795':'TM','800':'UG','804':'UA','784':'AE','826':'GB','840':'US','858':'UY','860':'UZ',
		'862':'VE','704':'VN','887':'YE','894':'ZM','716':'ZW','10':'AQ','112':'BY',
		'20':'AD','174':'KM','242':'FJ','296':'KI','583':'FM','520':'NR','585':'PW','882':'WS',
		'90':'SB','776':'TO','548':'VU','275':'PS','-99':'CY'
	};

	let mapRect = { ox: 0, oy: 0, mw: 0, mh: 0 };

	function projectLng(lng: number): number { return mapRect.ox + ((lng + 180) / 360) * mapRect.mw; }
	function projectLat(lat: number): number { return mapRect.oy + ((90 - lat) / 180) * mapRect.mh; }

	function calcMapRect(w: number, h: number): void {
		// Fit 2:1 equirectangular map within container, centered
		const containerRatio = w / h;
		const mapRatio = 2;
		let mw: number, mh: number;
		if (containerRatio > mapRatio) {
			mh = h; mw = h * mapRatio;
		} else {
			mw = w; mh = w / mapRatio;
		}
		mapRect = { ox: (w - mw) / 2, oy: (h - mh) / 2, mw, mh };
	}

	function tracePath(ctx: CanvasRenderingContext2D, coords: number[][][] | number[][][][], type: string): void {
		ctx.beginPath();
		const rings: number[][][] = type === 'Polygon' ? coords as number[][][] : type === 'MultiPolygon' ? (coords as number[][][][]).flat() : [];
		for (const ring of rings) {
			let moved = false;
			for (let i = 0; i < ring.length; i++) {
				const x = projectLng(ring[i][0]);
				const y = projectLat(ring[i][1]);
				if (i > 0 && Math.abs(ring[i][0] - ring[i-1][0]) > 90) {
					ctx.moveTo(x, y);
				} else if (!moved) {
					ctx.moveTo(x, y); moved = true;
				} else {
					ctx.lineTo(x, y);
				}
			}
			ctx.closePath();
		}
	}

	function getColor(ratio: number): string | null {
		if (ratio <= 0) return null;
		// Blue ramp: low → dim accent, high → bright accent
		const r = Math.round(30 + ratio * 78);
		const g = Math.round(40 + ratio * 102);
		const b = Math.round(90 + ratio * 149);
		return `rgb(${r}, ${g}, ${b})`;
	}

	function draw(): void {
		if (!canvas) return;
		const ctx = canvas.getContext('2d');
		if (!ctx) return;
		const dpr = window.devicePixelRatio || 1;
		const parent = canvas.parentElement;
		if (!parent) return;
		const w = parent.clientWidth;
		const h = parent.clientHeight;
		if (w === 0 || h === 0) return;
		canvas.width = w * dpr;
		canvas.height = h * dpr;
		canvas.style.width = w + 'px';
		canvas.style.height = h + 'px';
		ctx.scale(dpr, dpr);

		// Background
		ctx.fillStyle = '#0d0f17';
		ctx.fillRect(0, 0, w, h);

		if (!worldGeo || !worldGeo.features) return;

		calcMapRect(w, h);

		// Aggregate visitors by alpha-2 country code
		const countryMap: Record<string, CountryData> = {};
		let maxReqs = 0;
		for (const v of visitors) {
			if (!v.country_code) continue;
			const cc = v.country_code.toUpperCase();
			if (!countryMap[cc]) countryMap[cc] = { count: 0, country: v.country, code: cc };
			countryMap[cc].count += v.request_count;
			maxReqs = Math.max(maxReqs, countryMap[cc].count);
		}
		if (maxReqs === 0) maxReqs = 1;

		countryHitAreas = [];
		const baseFill = '#1a1d30';
		const borderColor = '#2e3145';
		const activeBorder = '#4a5280';

		// Draw each country
		for (const feat of worldGeo.features) {
			const numId = String(feat.id);
			const alpha2 = numToAlpha2[numId];
			const coords = feat.geometry.coordinates;
			const type = feat.geometry.type;
			const data = alpha2 ? countryMap[alpha2] : null;

			if (data) {
				const ratio = Math.pow(data.count / maxReqs, 0.45);
				ctx.fillStyle = getColor(ratio) || baseFill;
				ctx.strokeStyle = activeBorder;
				ctx.lineWidth = 0.8;
			} else {
				ctx.fillStyle = baseFill;
				ctx.strokeStyle = borderColor;
				ctx.lineWidth = 0.5;
			}

			tracePath(ctx, coords, type);
			ctx.fill();
			ctx.stroke();

			// Store hit area for hover detection
			if (alpha2) {
				countryHitAreas.push({ alpha2, coords, type, data: data || null });
			}
		}
	}

	function hitTest(mx: number, my: number): HitArea | null {
		if (!canvas) return null;
		const ctx = canvas.getContext('2d');
		if (!ctx) return null;
		const dpr = window.devicePixelRatio || 1;
		for (const area of countryHitAreas) {
			if (!area.data) continue;
			tracePath(ctx, area.coords, area.type);
			if (ctx.isPointInPath(mx * dpr, my * dpr)) return area;
		}
		return null;
	}

	function handleHover(e: MouseEvent): void {
		if (!canvas || countryHitAreas.length === 0) return;
		const rect = canvas.getBoundingClientRect();
		const mx = e.clientX - rect.left;
		const my = e.clientY - rect.top;

		const hit = hitTest(mx, my);
		if (hit && hit.data) {
			const safeCode = esc(hit.alpha2).toLowerCase().replace(/[^a-z]/g, '');
			const flag = safeCode ? `<img src="https://flagcdn.com/24x18/${safeCode}.png" width="18" height="13" style="vertical-align:middle;margin-right:6px"/>` : '';
			let html = `<div class="tooltip-row" style="gap:0.375rem">${flag}<span class="tooltip-label" style="min-width:0">${esc(hit.data.country || hit.alpha2)}</span></div>`;
			html += `<div class="tooltip-row"><span class="tooltip-dot" style="background:#6C8EEF"></span><span class="tooltip-label">Requests</span><span class="tooltip-val">${formatNumber(hit.data.count)}</span></div>`;
			let tx = e.clientX + 14, ty = e.clientY - 10;
			if (tx + 180 > window.innerWidth - 10) tx = e.clientX - 194;
			if (ty < 10) ty = 10;
			onTooltip({ visible: true, x: tx, y: ty, html });
			canvas.style.cursor = 'pointer';
		} else {
			onTooltip({ visible: false });
			canvas.style.cursor = 'default';
		}
	}

	function handleLeave(): void {
		onTooltip({ visible: false });
		if (canvas) canvas.style.cursor = 'default';
	}

	function init(node: HTMLCanvasElement): { destroy: () => void } {
		canvas = node;
		resizeObserver = new ResizeObserver(() => { draw(); });
		resizeObserver.observe(node.parentElement!);
		draw();
		return { destroy() { if (resizeObserver) resizeObserver.disconnect(); canvas = null; } };
	}

	$effect(() => {
		// Track reactive dependencies
		visitors;
		worldGeo;
		if (canvas) {
			setTimeout(() => draw(), 50);
		}
	});
</script>

<canvas use:init onmousemove={handleHover} onmouseleave={handleLeave} aria-label="World map showing visitor locations by country"></canvas>
