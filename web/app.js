'use strict';

// ── SVG Icon Library ───────────────────────────────────────────────────────
const ICONS = {
  trophy: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9H4a2 2 0 0 1-2-2V5h4"/><path d="M18 9h2a2 2 0 0 0 2-2V5h-4"/><path d="M8 21h8"/><path d="M12 17v4"/><path d="M6 9a6 6 0 0 0 12 0V3H6z"/></svg>`,
  ratio:  `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v18h18"/><polyline points="7 12 10 9 13 12 17 8"/></svg>`,
  speed:  `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0-18 0"/><polyline points="12 7 12 12 15 15"/></svg>`,
  star:   `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>`,
  bug:    `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2l1.88 1.88"/><path d="M14.12 3.88 16 2"/><path d="M9 7.13v-1a3.003 3.003 0 1 1 6 0v1"/><path d="M12 20c-3.3 0-6-2.7-6-6v-3a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v3c0 3.3-2.7 6-6 6z"/><path d="M12 20v-9"/><path d="M6.53 9C4.6 8.8 3 7.1 3 5"/><path d="M6 13H2"/><path d="M3 21c0-2.1 1.7-3.9 4-4"/><path d="M17.47 9c1.93-.2 3.53-1.9 3.53-4"/><path d="M18 13h4"/><path d="M21 21c0-2.1-1.7-3.9-4-4"/></svg>`,
  download:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>`,
  file:   `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5z"/><polyline points="14 2 14 8 20 8"/></svg>`,
  image:  `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>`,
  globe:  `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>`,
};
function svgIcon(name) { return ICONS[name] || ''; }

// ── State ──────────────────────────────────────────────────────────────────
let currentImageID      = null;
let currentExportID     = null;
let originalObjectURL   = null;
let lastBenchmarkResults = [];
let _origPixels = null;   // cached for heatmap redraws
let _compPixels = null;
let _heatW = 0, _heatH = 0;

// ── Init Icons ─────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  const set = (id, icon) => { const el = document.getElementById(id); if (el) el.innerHTML = svgIcon(icon); };
  set('dl-icon-dct',  'file');
  set('dl-icon-jpeg', 'image');
  set('dl-icon-webp', 'globe');
  const csv = document.getElementById('csv-btn');
  if (csv) csv.innerHTML = svgIcon('download') + ' CSV Report';
});

// ── Keyboard Shortcuts ─────────────────────────────────────────────────────
document.addEventListener('keydown', e => {
  if (e.ctrlKey || e.metaKey) {
    if (e.key === 'u') { e.preventDefault(); document.getElementById('file-input').click(); }
    if (e.key === 'Enter') { e.preventDefault(); document.getElementById('compress-btn').click(); }
    if (e.key === 's') { e.preventDefault(); downloadImage('dct'); }
  }
});

// ── Theme ──────────────────────────────────────────────────────────────────
function toggleTheme() {
  const isLight = document.body.classList.toggle('light');
  document.getElementById('theme-toggle').textContent = isLight ? 'Dark Mode' : 'Light Mode';
}

// ── Loading Overlay ────────────────────────────────────────────────────────
function showLoading(text, sub) {
  document.getElementById('loading-text').textContent = text || 'Processing…';
  document.getElementById('loading-sub').textContent  = sub  || '';
  document.getElementById('loading-overlay').classList.add('active');
}
function hideLoading() {
  document.getElementById('loading-overlay').classList.remove('active');
}

// ── Drag & Drop ────────────────────────────────────────────────────────────
function onDragOver(e) {
  e.preventDefault();
  document.getElementById('drop-zone').classList.add('drag-over');
}
function onDragLeave() {
  document.getElementById('drop-zone').classList.remove('drag-over');
}
function onDrop(e) {
  e.preventDefault();
  document.getElementById('drop-zone').classList.remove('drag-over');
  const file = e.dataTransfer.files[0];
  if (file) uploadFile(file);
}
function onFileSelected(e) {
  const file = e.target.files[0];
  if (file) uploadFile(file);
}

// ── Upload ─────────────────────────────────────────────────────────────────
async function uploadFile(file) {
  showLoading('Uploading…', file.name);
  const fd = new FormData();
  fd.append('image', file);

  try {
    const res  = await fetch('/api/upload', { method: 'POST', body: fd });
    const data = await res.json();
    hideLoading();

    if (!res.ok) { setStatus('Upload failed: ' + (data.error || res.statusText)); return; }

    currentImageID    = data.id;
    originalObjectURL = URL.createObjectURL(file);

    document.getElementById('original-preview').src = originalObjectURL;
    document.getElementById('original-info').textContent =
      `${data.width}×${data.height} · ${formatBytes(data.size_bytes)} · ${data.format.toUpperCase()}`;

    resetCompressedPanel();
    document.getElementById('comparison-card').style.display = '';
    document.getElementById('compress-btn').disabled  = false;
    document.getElementById('benchmark-btn').disabled = false;
    setStatus('Uploaded: ' + file.name);
  } catch (err) {
    hideLoading();
    setStatus('Upload error: ' + err.message);
  }
}

function resetCompressedPanel() {
  // Compressed image
  const compImg = document.getElementById('compressed-preview');
  compImg.style.display = 'none';
  compImg.src = '';
  document.getElementById('compressed-placeholder-wrap').style.display = '';
  document.getElementById('compressed-info').textContent = '';
  // Heatmap
  document.getElementById('heatmap-canvas').style.display  = 'none';
  document.getElementById('heatmap-placeholder').style.display = '';
  document.getElementById('heatmap-legend').style.display  = 'none';
  _origPixels = _compPixels = null;
  // Metrics
  document.getElementById('metrics-section').style.display = 'none';
  document.getElementById('metrics-grid').innerHTML = '';
  // Benchmark card
  document.getElementById('benchmark-card').style.display  = 'none';
  lastBenchmarkResults = [];
}

// ── Compress ───────────────────────────────────────────────────────────────
async function compressImage() {
  if (!currentImageID) return;

  const quality     = parseInt(document.getElementById('quality-slider').value, 10);
  const subsampling = document.getElementById('subsampling-select').value;

  showLoading('Compressing…', 'DCT transform → Quantize → Huffman encode');
  runProgressBar();

  try {
    const res  = await fetch('/api/compress', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ image_id: currentImageID, quality, chroma_subsampling: subsampling }),
    });
    const data = await res.json();
    hideLoading();

    if (!res.ok) { setStatus('Compression failed: ' + (data.error || res.statusText)); return; }

    // Show compressed JPEG preview, then generate heatmap on load
    if (data.preview_url) {
      const compImg = document.getElementById('compressed-preview');
      compImg.onload = () => {
        document.getElementById('compressed-placeholder-wrap').style.display = 'none';
        compImg.style.display = '';
        generateDiffHeatmap();
        addImageZoom();
      };
      compImg.src = data.preview_url;
    }

    const fr = data.file_ratio || 0;
    const frLabel = fr >= 1
      ? `${fr.toFixed(2)}x vs file`
      : `${(1/fr).toFixed(2)}x larger than file`;
    document.getElementById('compressed-info').textContent =
      `${formatBytes(data.compressed_size)} · ${frLabel}`;

    renderMetrics(data, quality, subsampling);
    document.getElementById('metrics-section').style.display = '';

    currentExportID = data.export_id || null;
    // Update DCT size label in download bar
    const dctLabel = document.getElementById('dl-size-dct');
    if (dctLabel) dctLabel.textContent = formatBytes(data.compressed_size) + ' — exact';

    setStatus('Compression complete.');
  } catch (err) {
    hideLoading();
    setStatus('Compression error: ' + err.message);
  }
}

// ── Progress Bar ──────────────────────────────────────────────────────────
function runProgressBar() {
  const container = document.getElementById('progress-container');
  const fill      = document.getElementById('progress-fill');
  const text      = document.getElementById('progress-text');
  container.style.display = 'block';
  fill.style.width = '0%';
  const steps = [
    [15,  'Reading image…'],
    [35,  'Applying DCT…'],
    [60,  'Quantizing…'],
    [80,  'Huffman encoding…'],
    [100, 'Done!'],
  ];
  let i = 0;
  const tick = setInterval(() => {
    if (i >= steps.length) { clearInterval(tick); setTimeout(() => { container.style.display = 'none'; }, 800); return; }
    fill.style.width = steps[i][0] + '%';
    text.textContent  = steps[i][1];
    i++;
  }, 220);
}

// ── Image Zoom ─────────────────────────────────────────────────────────────
function addImageZoom() {
  document.querySelectorAll('.image-panel img').forEach(img => {
    img.style.cursor = 'zoom-in';
    img.onclick = function() {
      if (!this.src || this.style.display === 'none') return;
      const modal = document.createElement('div');
      modal.className = 'zoom-modal';
      const zi = document.createElement('img');
      zi.src = this.src;
      modal.appendChild(zi);
      modal.onclick = () => modal.remove();
      document.body.appendChild(modal);
    };
  });
}

// ── Export / Download ──────────────────────────────────────────────────────
async function downloadImage(format) {
  if (!currentExportID) { setStatus('No compressed image to download.'); return; }

  if (format === 'webp') {
    const compImg = document.getElementById('compressed-preview');
    if (!compImg || !compImg.src || compImg.style.display === 'none') {
      setStatus('Compress an image first to download WebP.');
      return;
    }
    const canvas = document.createElement('canvas');
    canvas.width  = compImg.naturalWidth;
    canvas.height = compImg.naturalHeight;
    canvas.getContext('2d').drawImage(compImg, 0, 0);
    canvas.toBlob(blob => {
      if (!blob) { setStatus('WebP not supported in this browser.'); return; }
      const url = URL.createObjectURL(blob);
      Object.assign(document.createElement('a'), {
        href: url, download: `compressed_${currentExportID}.webp`,
      }).click();
      URL.revokeObjectURL(url);
    }, 'image/webp', 0.85);
    return;
  }

  const fmt = format || 'dct';
  const url = `/api/export?id=${currentExportID}&format=${fmt}`;
  const res = await fetch(url);
  if (!res.ok) { setStatus('Download failed.'); return; }
  const blob = await res.blob();
  const objURL = URL.createObjectURL(blob);
  Object.assign(document.createElement('a'), {
    href: objURL,
    download: `compressed_${currentExportID}.${fmt === 'jpeg' ? 'jpg' : 'dct'}`,
  }).click();
  URL.revokeObjectURL(objURL);
}

// ── Difference Heatmap ─────────────────────────────────────────────────────
function generateDiffHeatmap() {
  const origImg = document.getElementById('original-preview');
  const compImg = document.getElementById('compressed-preview');
  if (!origImg.complete || !compImg.complete) return;

  const w = origImg.naturalWidth;
  const h = origImg.naturalHeight;
  if (!w || !h) return;

  // Cap resolution at 800px wide to keep it fast on large images
  const scale = Math.min(1, 800 / w);
  const sw = Math.round(w * scale);
  const sh = Math.round(h * scale);

  try {
    const tmpA = Object.assign(document.createElement('canvas'), { width: sw, height: sh });
    tmpA.getContext('2d').drawImage(origImg, 0, 0, sw, sh);
    _origPixels = tmpA.getContext('2d').getImageData(0, 0, sw, sh);

    const tmpB = Object.assign(document.createElement('canvas'), { width: sw, height: sh });
    tmpB.getContext('2d').drawImage(compImg, 0, 0, sw, sh);
    _compPixels = tmpB.getContext('2d').getImageData(0, 0, sw, sh);

    _heatW = sw;
    _heatH = sh;
    paintHeatmap();
  } catch (e) {
    // Canvas taint or other issue — silently skip heatmap
    console.warn('Heatmap generation skipped:', e.message);
  }
}

// Called when amplification slider changes
function redrawHeatmap() {
  if (_origPixels && _compPixels) paintHeatmap();
}

function paintHeatmap() {
  const amp    = parseInt(document.getElementById('amplify-slider').value, 10);
  const canvas = document.getElementById('heatmap-canvas');
  canvas.width  = _heatW;
  canvas.height = _heatH;
  const ctx  = canvas.getContext('2d');
  const diff = ctx.createImageData(_heatW, _heatH);

  for (let i = 0; i < _origPixels.data.length; i += 4) {
    const dr  = Math.abs(_origPixels.data[i]   - _compPixels.data[i]);
    const dg  = Math.abs(_origPixels.data[i+1] - _compPixels.data[i+1]);
    const db  = Math.abs(_origPixels.data[i+2] - _compPixels.data[i+2]);
    const avg = (dr + dg + db) / 3;
    const v   = Math.min(255, avg * amp);

    // Green (low) → Yellow (medium) → Red (high)
    let r, g, b;
    if (v < 85) {
      r = Math.round(v * 3);
      g = 200;
      b = 0;
    } else if (v < 170) {
      r = 255;
      g = Math.round(200 * (1 - (v - 85) / 85));
      b = 0;
    } else {
      r = 255;
      g = 0;
      b = 0;
    }

    diff.data[i]   = r;
    diff.data[i+1] = g;
    diff.data[i+2] = b;
    diff.data[i+3] = 255;
  }

  ctx.putImageData(diff, 0, 0);
  canvas.style.display = '';
  document.getElementById('heatmap-placeholder').style.display = 'none';
  document.getElementById('heatmap-legend').style.display = '';
}

// ── Metric Cards ───────────────────────────────────────────────────────────
function qualityClass(psnr) {
  return psnr >= 40 ? 'excellent' : psnr >= 35 ? 'good' : psnr >= 30 ? 'acceptable' : 'poor';
}
function qualityLabel(psnr) {
  return psnr >= 40 ? 'Excellent' : psnr >= 35 ? 'Good' : psnr >= 30 ? 'Acceptable' : 'Poor';
}
function ssimClass(ssim) {
  return ssim >= 0.99 ? 'excellent' : ssim >= 0.95 ? 'good' : ssim >= 0.90 ? 'acceptable' : 'poor';
}
function ssimLabel(ssim) {
  return ssim >= 0.99 ? 'Excellent' : ssim >= 0.95 ? 'Good' : ssim >= 0.90 ? 'Acceptable' : 'Poor';
}

function renderMetrics(data) {
  const psnr      = data.psnr;
  const ssim      = data.ssim;
  const pc        = qualityClass(psnr);
  const sc        = ssimClass(ssim);
  const fileRatio = data.file_ratio || 0;
  const frCls     = fileRatio >= 2 ? 'excellent' : fileRatio >= 1 ? 'good' : 'poor';
  const frNote    = fileRatio >= 1
    ? fileRatio.toFixed(2) + 'x smaller than uploaded file'
    : 'DCT output larger than input (small/flat images compress poorly)';

  const tooltips = {
    'Uploaded File': 'Size of the original file you uploaded',
    'DCT Output':    'Size of the compressed .dct binary',
    'File Ratio':    'How many times smaller the .dct file is vs the uploaded file',
    'vs Raw RGB':    'Compression ratio vs uncompressed RGB pixel data',
    'PSNR':          'Peak Signal-to-Noise Ratio — higher is better (30+ ok, 40+ excellent)',
    'SSIM':          'Structural Similarity Index — closer to 1.0 means more similar to original',
    'Encode Time':   'Time taken to run the full DCT compression pipeline',
  };

  const cards = [
    { title: 'Uploaded File', value: formatBytes(data.file_original_size || data.original_size) },
    { title: 'DCT Output',    value: formatBytes(data.compressed_size) },
    {
      title: 'File Ratio',
      value: fileRatio.toFixed(2) + 'x',
      bar:   true,
      pct:   Math.min(100, (fileRatio / 10) * 100),
      cls:   frCls,
      note:  frNote,
    },
    {
      title: 'vs Raw RGB',
      value: data.compression_ratio.toFixed(2) + 'x',
      bar:   true,
      pct:   Math.min(100, (data.compression_ratio / 20) * 100),
      cls:   data.compression_ratio >= 10 ? 'excellent' : data.compression_ratio >= 5 ? 'good' : 'acceptable',
      note:  'ratio vs uncompressed ' + formatBytes(data.original_size),
    },
    {
      title: 'PSNR',
      value: isFinite(psnr) ? psnr.toFixed(2) + ' dB' : '∞',
      bar:   true,
      pct:   Math.min(100, (psnr / 50) * 100),
      cls:   pc,
      note:  qualityLabel(psnr) + ' · 30+ ok, 40+ excellent',
    },
    {
      title: 'SSIM',
      value: ssim.toFixed(4),
      bar:   true,
      pct:   ssim * 100,
      cls:   sc,
      note:  ssimLabel(ssim) + ' · 1.0 = identical',
    },
    {
      title: 'Encode Time',
      value: data.encoding_time_ms + ' ms',
    },
  ];

  document.getElementById('metrics-grid').innerHTML = cards.map(c => `
    <div class="metric-card">
      <h4 ${tooltips[c.title] ? `data-tooltip="${tooltips[c.title]}"` : ''}>${c.title}</h4>
      <div class="metric-value">${c.value}</div>
      ${c.bar ? `
        <div class="metric-bar-bg">
          <div class="metric-bar-fill bar-${c.cls}" style="width:${c.pct.toFixed(1)}%"></div>
        </div>
        <div class="metric-label ${c.cls}">${c.note}</div>
      ` : ''}
    </div>
  `).join('');
}

// ── Benchmark ──────────────────────────────────────────────────────────────
async function runBenchmark() {
  if (!currentImageID) return;

  const quality = parseInt(document.getElementById('quality-slider').value, 10);
  showLoading('Running Benchmark…', 'Comparing all compression methods (Python may take a moment)');

  try {
    const res  = await fetch('/api/benchmark', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ image_id: currentImageID, quality }),
    });
    const data = await res.json();
    hideLoading();

    if (!res.ok) { setStatus('Benchmark failed: ' + (data.error || res.statusText)); return; }

    lastBenchmarkResults = data.results || [];
    renderBenchmarkTable(lastBenchmarkResults);
    document.getElementById('benchmark-card').style.display = '';
    setStatus('Benchmark complete — ' + lastBenchmarkResults.length + ' methods compared.');
  } catch (err) {
    hideLoading();
    setStatus('Benchmark error: ' + err.message);
  }
}

function renderBenchmarkTable(results) {
  if (!results || !results.length) {
    document.getElementById('benchmark-tbody').innerHTML =
      '<tr><td colspan="7" style="text-align:center;color:var(--text2);padding:1rem">No results</td></tr>';
    document.getElementById('bm-hero').innerHTML = '';
    document.getElementById('bm-insights').innerHTML = '';
    return;
  }

  const minSize  = Math.min(...results.map(r => r.CompressedSize));
  const maxRatio = Math.max(...results.map(r => r.CompressionRatio));
  const maxPSNR  = Math.max(...results.map(r => r.PSNR));
  const maxSSIM  = Math.max(...results.map(r => r.SSIM));
  const minTime  = Math.min(...results.map(r => r.EncodingTime));
  const maxSize  = Math.max(...results.map(r => r.CompressedSize));

  const trophy = c => c ? ' <span class="trophy" title="Best">' + svgIcon('trophy') + '</span>' : '';
  const pClass = psnr => psnr >= 40 ? 'excellent' : psnr >= 35 ? 'good' : psnr >= 30 ? 'acceptable' : 'poor';
  const badge = r => {
    if (r.Method === 'DCTPress')       return '<span class="bench-badge bench-badge-yours">My Work</span>';
    if (r.Language === 'Python')       return '<span class="bench-badge bench-badge-python">Python</span>';
    return '<span class="bench-badge bench-badge-baseline">Baseline</span>';
  };

  document.getElementById('benchmark-tbody').innerHTML = results.map(r => {
    const encMs  = Math.round(r.EncodingTime / 1e6);
    const barPct = Math.round((r.CompressedSize / maxSize) * 100);
    return `
      <tr class="${r.Method === 'DCTPress' ? 'bench-row-yours' : ''}">
        <td><strong>${r.Method}</strong> ${badge(r)}</td>
        <td>${r.Language}</td>
        <td>
          ${formatBytes(r.CompressedSize)}${trophy(r.CompressedSize === minSize)}
          <div class="bench-size-bar"><div class="bench-size-fill" style="width:${barPct}%"></div></div>
        </td>
        <td>${r.CompressionRatio.toFixed(2)}x${trophy(r.CompressionRatio === maxRatio)}</td>
        <td class="${pClass(r.PSNR)}">${r.PSNR.toFixed(2)} dB${trophy(r.PSNR === maxPSNR)}</td>
        <td>${r.SSIM.toFixed(4)}${trophy(r.SSIM === maxSSIM)}</td>
        <td>${encMs} ms${trophy(r.EncodingTime === minTime)}</td>
      </tr>`;
  }).join('');

  // ── Hero Stats ──
  const yours  = results.find(r => r.Method === 'DCTPress');
  const goJpeg = results.find(r => r.Method === 'Go stdlib JPEG');
  if (yours && goJpeg && goJpeg.CompressedSize > 0 && goJpeg.EncodingTime > 0) {
    const pctDiff    = ((goJpeg.CompressedSize - yours.CompressedSize) / goJpeg.CompressedSize * 100);
    const speedRatio = (yours.EncodingTime / goJpeg.EncodingTime).toFixed(1);
    const winner     = pctDiff >= 0 ? 'DCTPress' : 'Go stdlib JPEG';
    document.getElementById('bm-hero').innerHTML = `
      <div class="bm-hero-stat">
        <div class="bm-hero-icon">${svgIcon('trophy')}</div>
        <div>
          <div class="bm-hero-label">Best Compression</div>
          <div class="bm-hero-value">${winner}</div>
          <div class="bm-hero-detail">${Math.abs(pctDiff).toFixed(1)}% ${pctDiff >= 0 ? 'smaller than stdlib' : 'larger than stdlib'}</div>
        </div>
      </div>
      <div class="bm-hero-stat">
        <div class="bm-hero-icon">${svgIcon('ratio')}</div>
        <div>
          <div class="bm-hero-label">DCTPress Ratio</div>
          <div class="bm-hero-value">${yours.CompressionRatio.toFixed(2)}x</div>
          <div class="bm-hero-detail">vs ${goJpeg.CompressionRatio.toFixed(2)}x baseline</div>
        </div>
      </div>
      <div class="bm-hero-stat">
        <div class="bm-hero-icon">${svgIcon('speed')}</div>
        <div>
          <div class="bm-hero-label">Speed Trade-off</div>
          <div class="bm-hero-value">${speedRatio}x slower</div>
          <div class="bm-hero-detail">${Math.round(yours.EncodingTime/1e6)}ms vs ${Math.round(goJpeg.EncodingTime/1e6)}ms</div>
        </div>
      </div>`;

    // ── Insights ──
    const qualityDiff = (yours.PSNR - goJpeg.PSNR).toFixed(2);

    const pythonResults = results.filter(r => r.Language === 'Python');
    const avgGoRatio    = results.filter(r => r.Language === 'Go')
                                  .reduce((s, r) => s + r.CompressionRatio, 0) / 2;
    const buggyPython   = pythonResults.filter(r => r.CompressionRatio < avgGoRatio * 0.1);

    const bugInsight = buggyPython.length ? `
      <div class="bm-insight bm-insight-bug">
        <div class="bm-insight-icon">${svgIcon('bug')}</div>
        <div><strong>Python Ratio Bug Detected</strong>
          <p>${buggyPython.map(r => r.Method).join(', ')} show(s) a ratio of ${buggyPython.map(r => r.CompressionRatio.toFixed(2) + 'x').join(', ')} — impossible. The script compares compressed input vs compressed output instead of raw RGB. Fix: use <code>width × height × 3</code> as original size.</p>
        </div>
      </div>` : '';

    document.getElementById('bm-insights').innerHTML = `
      <div class="bm-insight bm-insight-win">
        <div class="bm-insight-icon">${svgIcon('star')}</div>
        <div><strong>Your Advantage</strong>
          <p>DCTPress is ${Math.abs(pctDiff).toFixed(1)}% ${pctDiff >= 0 ? 'smaller' : 'larger'} than Go stdlib JPEG with only ${Math.abs(qualityDiff)} dB PSNR difference.</p>
        </div>
      </div>
      <div class="bm-insight bm-insight-tradeoff">
        <div class="bm-insight-icon">${svgIcon('speed')}</div>
        <div><strong>Worth the Wait?</strong>
          <p>${speedRatio}x slower encoding — worth it for archival use, not for real-time applications.</p>
        </div>
      </div>${bugInsight}`;
  } else {
    document.getElementById('bm-hero').innerHTML = '';
    document.getElementById('bm-insights').innerHTML = '';
  }

  // remove old summary strip if present
  const old = document.getElementById('bench-summary');
  if (old) old.remove();
}

// ── CSV Export ─────────────────────────────────────────────────────────────
function downloadCSV() {
  if (!lastBenchmarkResults.length) { setStatus('No benchmark data to export.'); return; }

  const headers = ['Method', 'Language', 'CompressedSize_bytes', 'CompressionRatio',
                   'PSNR_dB', 'SSIM', 'EncodeTime_ms'];
  const rows = lastBenchmarkResults.map(r => [
    r.Method, r.Language,
    r.CompressedSize,
    r.CompressionRatio.toFixed(4),
    r.PSNR.toFixed(4),
    r.SSIM.toFixed(6),
    Math.round(r.EncodingTime / 1e6),
  ]);

  const csv = [headers, ...rows].map(row => row.join(',')).join('\n');
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }));
  Object.assign(document.createElement('a'), {
    href: url, download: 'benchmark_report.csv'
  }).click();
  URL.revokeObjectURL(url);
}

// ── Helpers ────────────────────────────────────────────────────────────────
function formatBytes(bytes) {
  if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
  if (bytes >= 1024)        return (bytes / 1024).toFixed(1) + ' KB';
  return bytes + ' B';
}

function setStatus(msg) {
  document.getElementById('upload-status').textContent = msg;
}
