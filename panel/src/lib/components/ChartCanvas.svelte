<script>
	import { chartLabels, formatBytes, formatNumber, formatMs, formatTime, formatFullTime, esc } from '$lib/metricsUtils';

	export let id = '';
	export let data = [];
	export let keys = [];
	export let colors = [];
	export let type = 'line';
	export let formatter = null;
	export let selectedRange = '24h';
	export let onTooltip = () => {};

	let canvas;
	let meta = {};

	function fmtVal(val) {
		if (formatter === 'bytes') return formatBytes(val);
		if (formatter === 'ms') return formatMs(val);
		return formatNumber(Math.round(val));
	}

	function draw() {
		if (!canvas || !data || data.length === 0) return;
		const ctx = canvas.getContext('2d');
		const dpr = window.devicePixelRatio || 1;
		const rect = canvas.getBoundingClientRect();
		canvas.width = rect.width * dpr; canvas.height = rect.height * dpr;
		ctx.scale(dpr, dpr);
		const w = rect.width, h = rect.height;
		const pad = { top: 14, right: 14, bottom: 28, left: 52 };
		const cW = w - pad.left - pad.right, cH = h - pad.top - pad.bottom;

		let maxVal = 0;
		for (const d of data) {
			if (type === 'stacked') { let s = 0; for (const k of keys) s += (d[k]||0); maxVal = Math.max(maxVal, s); }
			else { for (const k of keys) maxVal = Math.max(maxVal, d[k]||0); }
		}
		if (maxVal === 0) maxVal = 1;

		meta = { data, keys, colors, type, formatter, padding: pad, chartW: cW, chartH: cH, w, h, maxVal };
		ctx.clearRect(0, 0, w, h);

		drawGrid(ctx, pad, cW, cH, h, maxVal);
		drawXLabels(ctx, pad, cW, h);

		if (type === 'stacked') drawStacked(ctx, pad, cW, cH, maxVal);
		else drawLines(ctx, pad, cW, cH, maxVal);
	}

	function drawGrid(ctx, pad, cW, cH, h, maxVal) {
		const gridN = 4;
		for (let i = 0; i <= gridN; i++) {
			const y = Math.round(pad.top + (cH / gridN) * i) + 0.5;
			ctx.strokeStyle = '#2e3145'; ctx.lineWidth = 1;
			ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + cW, y); ctx.stroke();
			ctx.fillStyle = '#6b6f88'; ctx.font = '14px "DM Sans", system-ui'; ctx.textAlign = 'right';
			ctx.fillText(fmtVal(maxVal * (1 - i / gridN)), pad.left - 8, y + 4);
		}
	}

	function drawXLabels(ctx, pad, cW, h) {
		ctx.fillStyle = '#6b6f88'; ctx.font = '13px "DM Sans", system-ui'; ctx.textAlign = 'center';
		const sLbl = formatTime(data[0].time, selectedRange);
		const lW = ctx.measureText(sLbl).width + 16;
		const mL = Math.max(2, Math.floor(cW / lW)), lStep = Math.max(1, Math.ceil(data.length / mL));
		for (let i = 0; i < data.length; i += lStep) {
			const x = pad.left + (i / (data.length - 1 || 1)) * cW;
			ctx.fillText(formatTime(data[i].time, selectedRange), x, h - 6);
		}
		const lastD = Math.floor((data.length-1)/lStep)*lStep;
		if (lastD !== data.length-1) {
			const lx = pad.left+cW, px = pad.left+(lastD/(data.length-1||1))*cW;
			if (lx-px>lW) ctx.fillText(formatTime(data[data.length-1].time, selectedRange), lx, h-6);
		}
	}

	function drawStacked(ctx, pad, cW, cH, maxVal) {
		const bW = Math.max(3, cW / data.length - 1);
		for (let ki = keys.length-1; ki >= 0; ki--) {
			ctx.fillStyle = colors[ki];
			for (let i = 0; i < data.length; i++) {
				let base = 0; for (let j = 0; j < ki; j++) base += (data[i][keys[j]]||0);
				const val = data[i][keys[ki]]||0;
				const x = pad.left + (i/(data.length-1||1))*cW - bW/2;
				const bH = ((base+val)/maxVal)*cH, baseH = (base/maxVal)*cH;
				ctx.beginPath(); ctx.roundRect(x, pad.top+cH-bH, bW, bH-baseH, 1); ctx.fill();
			}
		}
	}

	function drawLines(ctx, pad, cW, cH, maxVal) {
		for (let ki = 0; ki < keys.length; ki++) {
			ctx.fillStyle = colors[ki] + '15';
			ctx.beginPath();
			for (let i = 0; i < data.length; i++) {
				const x = pad.left + (i/(data.length-1||1))*cW;
				const y = pad.top + cH - ((data[i][keys[ki]]||0)/maxVal)*cH;
				i === 0 ? ctx.moveTo(x,y) : ctx.lineTo(x,y);
			}
			ctx.lineTo(pad.left+cW, pad.top+cH); ctx.lineTo(pad.left, pad.top+cH);
			ctx.closePath(); ctx.fill();
			ctx.strokeStyle = colors[ki]; ctx.lineWidth = 1.5; ctx.beginPath();
			for (let i = 0; i < data.length; i++) {
				const x = pad.left + (i/(data.length-1||1))*cW;
				const y = pad.top + cH - ((data[i][keys[ki]]||0)/maxVal)*cH;
				i === 0 ? ctx.moveTo(x,y) : ctx.lineTo(x,y);
			}
			ctx.stroke();
		}
	}

	function redraw(m, hoverIdx = -1) {
		if (!canvas) return;
		const { data: d, keys: k, colors: cc, type: t, padding: pad, chartW: cW, chartH: cH, w, h, maxVal } = m;
		const ctx = canvas.getContext('2d');
		ctx.save(); ctx.setTransform(1,0,0,1,0,0); ctx.clearRect(0,0,canvas.width,canvas.height); ctx.restore();

		drawGrid(ctx, pad, cW, cH, h, maxVal);
		drawXLabels(ctx, pad, cW, h);

		if (t === 'stacked') drawStacked(ctx, pad, cW, cH, maxVal);
		else drawLines(ctx, pad, cW, cH, maxVal);

		if (hoverIdx >= 0 && hoverIdx < d.length) {
			const hx = pad.left+(hoverIdx/(d.length-1||1))*cW;
			ctx.strokeStyle='#3d415a'; ctx.lineWidth=1; ctx.setLineDash([4,3]);
			ctx.beginPath(); ctx.moveTo(hx,pad.top); ctx.lineTo(hx,pad.top+cH); ctx.stroke(); ctx.setLineDash([]);
			for (let ki=0;ki<k.length;ki++) {
				const val=d[hoverIdx][k[ki]]||0;
				let hy;
				if (t==='stacked') { let s=0; for (let j=0;j<=ki;j++) s+=(d[hoverIdx][k[j]]||0); hy=pad.top+cH-(s/maxVal)*cH; }
				else { hy=pad.top+cH-(val/maxVal)*cH; }
				ctx.fillStyle='#1c1e2b'; ctx.beginPath(); ctx.arc(hx,hy,4.5,0,Math.PI*2); ctx.fill();
				ctx.strokeStyle=cc[ki]; ctx.lineWidth=2; ctx.stroke();
			}
		}
	}

	function handleHover(e) {
		if (!meta.data) return;
		const rect = canvas.getBoundingClientRect(), mouseX = e.clientX - rect.left;
		const { data: d, keys: k, colors: cc, padding: pad, chartW: cW } = meta;
		const relX = mouseX - pad.left;
		if (relX < -10 || relX > cW + 10 || d.length === 0) { onTooltip({ visible: false }); redraw(meta); return; }
		const idx = Math.max(0, Math.min(d.length-1, Math.round((relX/cW)*(d.length-1))));
		const pt = d[idx];
		let html = `<div class="tooltip-time">${esc(formatFullTime(pt.time))}</div>`;
		for (let ki = 0; ki < k.length; ki++) {
			const val = pt[k[ki]]||0, label = chartLabels[k[ki]]||k[ki];
			html += `<div class="tooltip-row"><span class="tooltip-dot" style="background:${cc[ki]}"></span><span class="tooltip-label">${esc(label)}</span><span class="tooltip-val">${esc(fmtVal(val))}</span></div>`;
		}
		let tx = e.clientX+14, ty = e.clientY-10;
		if (tx+200>window.innerWidth-10) tx = e.clientX-214;
		if (ty+36+k.length*28>window.innerHeight-10) ty = window.innerHeight-36-k.length*28-10;
		if (ty<10) ty=10;
		onTooltip({ visible: true, x: tx, y: ty, html });
		redraw(meta, idx);
	}

	function handleLeave() {
		onTooltip({ visible: false });
		if (meta.data) redraw(meta);
	}

	$: if (canvas && data && data.length > 0 && selectedRange) {
		setTimeout(() => draw(), 0);
	}
</script>

<canvas bind:this={canvas} {id} on:mousemove={handleHover} on:mouseleave={handleLeave} aria-label="Chart showing {keys.map(k => chartLabels[k] || k).join(', ')} data"></canvas>
