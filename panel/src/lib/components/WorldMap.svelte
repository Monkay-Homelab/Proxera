<script>
	import { countryCentroids, formatNumber, esc } from '$lib/metricsUtils';

	export let visitors = [];
	export let worldGeo = null;
	export let onTooltip = () => {};

	let canvas;
	let mapDots = [];

	function projectLng(lng, w) { return ((lng + 180) / 360) * w; }
	function projectLat(lat, h) { return ((90 - lat) / 180) * h; }

	function drawGeoShape(ctx, coords, type, w, h) {
		if (type === 'Polygon') {
			for (const ring of coords) drawRing(ctx, ring, w, h);
		} else if (type === 'MultiPolygon') {
			for (const polygon of coords) {
				for (const ring of polygon) drawRing(ctx, ring, w, h);
			}
		}
	}

	function drawRing(ctx, ring, w, h) {
		ctx.beginPath();
		let moved = false;
		for (let i = 0; i < ring.length; i++) {
			const x = projectLng(ring[i][0], w);
			const y = projectLat(ring[i][1], h);
			if (i > 0 && Math.abs(ring[i][0] - ring[i-1][0]) > 90) {
				ctx.moveTo(x, y);
			} else if (!moved) {
				ctx.moveTo(x, y);
				moved = true;
			} else {
				ctx.lineTo(x, y);
			}
		}
		ctx.closePath();
		ctx.fill();
		ctx.stroke();
	}

	function draw() {
		if (!canvas) return;
		const ctx = canvas.getContext('2d');
		const dpr = window.devicePixelRatio || 1;
		const parent = canvas.parentElement;
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

		// Draw land from GeoJSON
		if (worldGeo) {
			ctx.fillStyle = '#242a48';
			ctx.strokeStyle = '#333c5c';
			ctx.lineWidth = 0.5;
			for (const geom of worldGeo.features || [worldGeo]) {
				const coords = geom.geometry ? geom.geometry.coordinates : geom.coordinates;
				const type = geom.geometry ? geom.geometry.type : geom.type;
				drawGeoShape(ctx, coords, type, w, h);
			}
		}

		// Aggregate visitors by country
		const countryMap = {};
		let maxReqs = 0;
		for (const v of visitors) {
			if (!v.country_code) continue;
			const cc = v.country_code.toUpperCase();
			if (!countryMap[cc]) countryMap[cc] = { count: 0, country: v.country, code: cc };
			countryMap[cc].count += v.request_count;
			maxReqs = Math.max(maxReqs, countryMap[cc].count);
		}
		if (maxReqs === 0) maxReqs = 1;

		// Draw visitor heat dots and store positions
		mapDots = [];
		for (const cc of Object.keys(countryMap)) {
			const centroid = countryCentroids[cc];
			if (!centroid) continue;
			const [lat, lng] = centroid;
			const x = projectLng(lng, w);
			const y = projectLat(lat, h);
			const ratio = countryMap[cc].count / maxReqs;
			const radius = 4 + ratio * 18;
			const alpha = 0.25 + ratio * 0.55;

			mapDots.push({ x, y, radius, country: countryMap[cc].country, code: cc, count: countryMap[cc].count });

			// Glow
			const grad = ctx.createRadialGradient(x, y, 0, x, y, radius * 2.5);
			grad.addColorStop(0, `rgba(108, 142, 239, ${alpha * 0.5})`);
			grad.addColorStop(1, 'rgba(108, 142, 239, 0)');
			ctx.fillStyle = grad;
			ctx.beginPath();
			ctx.arc(x, y, radius * 2.5, 0, Math.PI * 2);
			ctx.fill();

			// Core dot
			ctx.fillStyle = `rgba(108, 142, 239, ${alpha + 0.2})`;
			ctx.beginPath();
			ctx.arc(x, y, radius, 0, Math.PI * 2);
			ctx.fill();

			// Bright center
			ctx.fillStyle = `rgba(160, 190, 255, ${alpha + 0.3})`;
			ctx.beginPath();
			ctx.arc(x, y, Math.max(2, radius * 0.35), 0, Math.PI * 2);
			ctx.fill();
		}
	}

	function handleHover(e) {
		if (!canvas || mapDots.length === 0) return;
		const rect = canvas.getBoundingClientRect();
		const mx = e.clientX - rect.left;
		const my = e.clientY - rect.top;
		let hit = null;
		for (const dot of mapDots) {
			const dist = Math.sqrt((mx - dot.x) ** 2 + (my - dot.y) ** 2);
			if (dist < Math.max(dot.radius, 10)) { hit = dot; break; }
		}
		if (hit) {
			const safeCode = esc(hit.code).toLowerCase().replace(/[^a-z]/g, '');
			const flag = safeCode ? `<img src="https://flagcdn.com/24x18/${safeCode}.png" width="18" height="13" style="vertical-align:middle;margin-right:6px"/>` : '';
			let html = `<div class="tooltip-row" style="gap:0.375rem">${flag}<span class="tooltip-label" style="min-width:0">${esc(hit.country || hit.code)}</span></div>`;
			html += `<div class="tooltip-row"><span class="tooltip-dot" style="background:#6C8EEF"></span><span class="tooltip-label">Requests</span><span class="tooltip-val">${formatNumber(hit.count)}</span></div>`;
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

	function handleLeave() {
		onTooltip({ visible: false });
		if (canvas) canvas.style.cursor = 'default';
	}

	function init(node) {
		canvas = node;
		draw();
		return { destroy() { canvas = null; } };
	}

	$: if (canvas && (visitors || worldGeo)) {
		setTimeout(() => draw(), 50);
	}
</script>

<canvas use:init on:mousemove={handleHover} on:mouseleave={handleLeave} aria-label="World map showing visitor locations by country"></canvas>
