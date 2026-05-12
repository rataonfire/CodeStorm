import { useState, useEffect, useRef, useCallback } from 'react'

const API = import.meta.env.VITE_API_URL || ''

async function apiFetch(path, opts) {
  const res = await fetch(API + path, opts)
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

function fmtAmount(n) {
  if (n == null) return '—'
  return Number(n).toLocaleString('ru-RU') + ' UZS'
}

function fmtAge(iso) {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60000) return Math.round(ms / 1000) + ' с назад'
  if (ms < 3600000) return Math.round(ms / 60000) + ' мин назад'
  return Math.round(ms / 3600000) + ' ч назад'
}

function useFadeUp() {
  useEffect(() => {
    const io = new IntersectionObserver(entries => {
      entries.forEach(e => { if (e.isIntersecting) e.target.classList.add('vis') })
    }, { threshold: 0.12 })
    document.querySelectorAll('.fade-up').forEach(el => io.observe(el))
    return () => io.disconnect()
  }, [])
}

function useReconWS(onMessage) {
  const [live, setLive] = useState(false)
  const wsRef = useRef(null)
  const cbRef = useRef(onMessage)
  cbRef.current = onMessage

  useEffect(() => {
    const wsBase = API.replace(/^http/, 'ws') || (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host
    let closed = false
    let timer

    function connect() {
      if (closed) return
      const ws = new WebSocket(wsBase + '/api/v1/ws')
      wsRef.current = ws

      ws.onopen = () => setLive(true)
      ws.onclose = () => {
        setLive(false)
        if (!closed) timer = setTimeout(connect, 3000)
      }
      ws.onerror = () => ws.close()
      ws.onmessage = e => {
        try { cbRef.current(JSON.parse(e.data)) } catch {}
      }
    }

    connect()
    return () => {
      closed = true
      clearTimeout(timer)
      wsRef.current?.close()
    }
  }, [])

  return live
}

function MismatchChart({ data }) {
  const canvasRef = useRef(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const dpr = window.devicePixelRatio || 1
    const W = canvas.parentElement.offsetWidth
    const H = 260
    canvas.width = W * dpr
    canvas.height = H * dpr
    canvas.style.width = W + 'px'
    canvas.style.height = H + 'px'
    const ctx = canvas.getContext('2d')
    ctx.scale(dpr, dpr)

    const points = data.length > 0 ? data.map(d => d.mismatch_count) : Array(20).fill(0)
    const maxV = Math.max(...points, 5)
    const pL = 52, pR = 20, pT = 16, pB = 36
    const cW = W - pL - pR, cH = H - pT - pB

    const xOf = i => pL + (cW / (points.length - 1)) * i
    const yOf = v => pT + cH - (v / maxV) * cH

    ctx.strokeStyle = 'rgba(255,255,255,0.07)'
    ctx.lineWidth = 1
    for (let i = 0; i <= 5; i++) {
      const y = pT + (cH / 5) * i
      ctx.beginPath(); ctx.moveTo(pL, y); ctx.lineTo(W - pR, y); ctx.stroke()
      ctx.fillStyle = 'rgba(255,255,255,0.35)'
      ctx.font = '11px Inter,sans-serif'
      ctx.textAlign = 'right'
      ctx.fillText(String(Math.round(maxV - (maxV / 5) * i)), pL - 6, y + 4)
    }

    ctx.textAlign = 'center'; ctx.fillStyle = 'rgba(255,255,255,0.35)'; ctx.font = '11px Inter,sans-serif'
    const step = Math.max(1, Math.floor(points.length / 6))
    for (let i = 0; i < points.length; i += step) {
      const label = data[i] ? new Date(data[i].minute).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : ''
      ctx.fillText(label, xOf(i), H - 8)
    }

    const g = ctx.createLinearGradient(0, pT, 0, H - pB)
    g.addColorStop(0, 'rgba(11,110,79,0.45)')
    g.addColorStop(1, 'rgba(11,110,79,0.02)')
    ctx.beginPath()
    points.forEach((v, i) => { i === 0 ? ctx.moveTo(xOf(i), yOf(v)) : ctx.lineTo(xOf(i), yOf(v)) })
    ctx.lineTo(xOf(points.length - 1), pT + cH)
    ctx.lineTo(pL, pT + cH); ctx.closePath()
    ctx.fillStyle = g; ctx.fill()

    ctx.beginPath(); ctx.strokeStyle = '#0B6E4F'; ctx.lineWidth = 2
    points.forEach((v, i) => { i === 0 ? ctx.moveTo(xOf(i), yOf(v)) : ctx.lineTo(xOf(i), yOf(v)) })
    ctx.stroke()

    points.forEach((v, i) => {
      if (v < maxV * 0.6) return
      const x = xOf(i), y = yOf(v)
      ctx.save(); ctx.setLineDash([4, 4]); ctx.strokeStyle = 'rgba(139,30,47,0.5)'; ctx.lineWidth = 1
      ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x, pT + cH); ctx.stroke(); ctx.restore()
      ctx.beginPath(); ctx.arc(x, y, 6, 0, Math.PI * 2); ctx.fillStyle = '#8B1E2F'; ctx.fill()
    })
  }, [data])

  return <canvas ref={canvasRef} id="chart" />
}

function MatcherSpeedometer({ avgLatency, logicLatency, successRate }) {
  const canvasRef = useRef(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    
    const dpr = window.devicePixelRatio || 1
    const W = 280
    const H = 280
    canvas.width = W * dpr
    canvas.height = H * dpr
    canvas.style.width = W + 'px'
    canvas.style.height = H + 'px'
    
    const ctx = canvas.getContext('2d')
    ctx.scale(dpr, dpr)
    ctx.clearRect(0, 0, W, H)

    const centerX = W / 2
    const centerY = H / 2
    const radius = 80

    ctx.fillStyle = 'rgba(11, 110, 79, 0.1)'
    ctx.beginPath()
    ctx.arc(centerX, centerY, radius + 15, 0, Math.PI * 2)
    ctx.fill()

    ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)'
    ctx.lineWidth = 20
    ctx.beginPath()
    ctx.arc(centerX, centerY, radius, Math.PI, Math.PI * 2, false)
    ctx.stroke()

    const normalizedLogic = Math.min(logicLatency / 10, 1)
    const endAngle = Math.PI + (Math.PI * normalizedLogic)
    
    let hue = logicLatency < 1 ? 120 : (logicLatency < 5 ? 50 : 0)
    
    ctx.strokeStyle = `hsl(${hue}, 100%, 50%)`
    ctx.lineWidth = 20
    ctx.lineCap = 'round'
    ctx.beginPath()
    ctx.arc(centerX, centerY, radius, Math.PI, endAngle, false)
    ctx.stroke()

    ctx.fillStyle = '#0B6E4F'
    ctx.beginPath()
    ctx.arc(centerX, centerY, 40, 0, Math.PI * 2)
    ctx.fill()

    ctx.fillStyle = '#FFF'
    ctx.font = 'bold 28px Inter, sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(logicLatency.toFixed(3), centerX, centerY - 8)

    ctx.fillStyle = 'rgba(255, 255, 255, 0.6)'
    ctx.font = '11px Inter, sans-serif'
    ctx.fillText('logic ms', centerX, centerY + 12)

    ctx.fillStyle = '#0B6E4F'
    ctx.font = 'bold 16px Inter, sans-serif'
    ctx.textAlign = 'center'
    ctx.fillText(`${successRate}% Match Rate`, centerX, centerY + 70)

  }, [logicLatency, successRate])

  return <canvas ref={canvasRef} id="speedometer" style={{ margin: '0 auto', display: 'block' }} />
}

function StatusBadge({ status }) {
  const map = {
    matched: ['b-ok', 'Matched'],
    mismatch: ['b-mis', 'Mismatch'],
    pending: ['b-pend', 'Pending'],
    degraded: ['b-off', 'Degraded'],
  }
  const [cls, label] = map[status] || ['b-pend', status]
  return <span className={`badge ${cls}`}>{label}</span>
}

function DemoSection() {
  const [tab, setTab] = useState('chart')
  const [transactions, setTransactions] = useState([])
  const [incidents, setIncidents] = useState([])
  const [totalIncidents, setTotalIncidents] = useState(0)
  const [chartData, setChartData] = useState([])
  const [sources, setSources] = useState({ merchant: 'unknown', gateway: 'unknown', bank: 'unknown' })
  const [activeIncident, setActiveIncident] = useState(null)
  const [actionLoading, setActionLoading] = useState(false)
  const [speedometer, setSpeedometer] = useState({ avg_latency_ms: 0, p50_latency_ms: 0, p99_latency_ms: 0, success_rate: 0, total_processed: 0, successful_matches: 0, failed_matches: 0 })

  const fetchAll = useCallback(async () => {
    try {
      const [txRes, incRes, chartRes, srcRes, speedRes] = await Promise.allSettled([
        apiFetch('/api/v1/transactions?limit=10'),
        apiFetch('/api/v1/incidents?status=open&limit=10'),
        apiFetch('/api/v1/metrics/mismatches-per-minute'),
        apiFetch('/api/v1/sources/health'),
        apiFetch('/api/v1/metrics/matcher-speedometer'),
      ])
      if (txRes.status === 'fulfilled') setTransactions(txRes.value.items || [])
      if (incRes.status === 'fulfilled') {
        const items = incRes.value.items || []
        setIncidents(items)
        setTotalIncidents(incRes.value.total_estimate || items.length)
        setActiveIncident(items[0] || null)
      }
      if (chartRes.status === 'fulfilled') setChartData(Array.isArray(chartRes.value) ? chartRes.value : [])
      if (srcRes.status === 'fulfilled') {
        const normalized = {}
        for (const [k, v] of Object.entries(srcRes.value)) {
          normalized[k] = typeof v === 'object' ? v.status : v
        }
        setSources(normalized)
      }
      if (speedRes.status === 'fulfilled') setSpeedometer(speedRes.value)
    } catch {}
  }, [])

  useEffect(() => { fetchAll() }, [fetchAll])
  
  useEffect(() => {
    const interval = setInterval(() => {
      apiFetch('/api/v1/metrics/matcher-speedometer').then(r => setSpeedometer(r)).catch(() => {})
      apiFetch('/api/v1/incidents?status=open&limit=1').then(r => setTotalIncidents(r.total_estimate || 0)).catch(() => {})
    }, 1000)
    return () => clearInterval(interval)
  }, [])

  const wsLive = useReconWS(useCallback(msg => {
    if (['transaction_received', 'transaction_matched', 'transaction_progress'].includes(msg.type)) {
      apiFetch('/api/v1/transactions?limit=10').then(r => setTransactions(r.items || [])).catch(() => {})
      apiFetch('/api/v1/metrics/mismatches-per-minute').then(r => setChartData(Array.isArray(r) ? r : [])).catch(() => {})
      apiFetch('/api/v1/metrics/matcher-speedometer').then(r => setSpeedometer(r)).catch(() => {})
    }
    if (['incident_created', 'incident_updated'].includes(msg.type)) {
      apiFetch('/api/v1/incidents?status=open&limit=10').then(r => {
        const items = r.items || []
        setIncidents(items)
        setTotalIncidents(r.total_estimate || items.length)
        setActiveIncident(prev => prev ? items.find(i => i.id === prev.id) || items[0] || null : items[0] || null)
      }).catch(() => {})
    }
    if (msg.type === 'source_status_changed') {
      setSources(prev => ({ ...prev, [msg.source]: msg.is_online ? 'online' : 'offline' }))
    }
  }, []))

  async function handleAck() {
    if (!activeIncident) return
    setActionLoading(true)
    try {
      await apiFetch(`/api/v1/incidents/${activeIncident.id}/ack`, { method: 'POST' })
      await fetchAll()
    } catch {}
    setActionLoading(false)
  }

  async function handleResolve() {
    if (!activeIncident) return
    setActionLoading(true)
    try {
      await apiFetch(`/api/v1/incidents/${activeIncident.id}/resolve`, { method: 'POST' })
      await fetchAll()
    } catch {}
    setActionLoading(false)
  }

  return (
    <section className="demo-sec" id="demo">
      <div className="wrap">
        <div className="sec-hd">
          <div className="sec-tag fade-up">Продукт</div>
          <h2 className="sec-h2 fade-up d1">
            Функциональная демонстрация
            <span className={`ws-pill ${wsLive ? 'live' : 'off'}`}>{wsLive ? 'LIVE' : 'OFFLINE'}</span>
          </h2>
          <p className="sec-sub fade-up d2">Так выглядит Chinor в работе — реальные транзакции, реальные системы, реальное время.</p>
        </div>

        <div className="src-health">
          {['merchant', 'gateway', 'bank'].map(src => (
            <span key={src} className={`src-dot ${sources[src] || 'offline'}`}>{src}</span>
          ))}
        </div>

        <div className="tabs fade-up">
          <button className={`tb ${tab === 'chart' ? 'on' : ''}`} onClick={() => setTab('chart')}>📈 График инцидентов</button>
          <button className={`tb ${tab === 'speedometer' ? 'on' : ''}`} onClick={() => setTab('speedometer')}>⚡ Спидометр матчера</button>
          <button className={`tb ${tab === 'table' ? 'on' : ''}`} onClick={() => setTab('table')}>📋 Интерфейс оператора</button>
          <button className={`tb ${tab === 'incident' ? 'on' : ''}`} onClick={() => setTab('incident')}>
            🔴 Инциденты {totalIncidents > 0 && <span style={{ background: '#8B1E2F', color: '#fff', borderRadius: '100px', padding: '1px 7px', fontSize: '11px', marginLeft: '6px' }}>{totalIncidents}</span>}
          </button>
        </div>

        <div className={`tab-p ${tab === 'chart' ? 'on' : ''}`}>
          <div className="ch-panel">
            <div className="ch-hd">
              <div>
                <div className="ch-title">Инциденты по минутам — последний час</div>
                <div className="ch-meta">Мониторинг: Merchant · Gateway · Bank</div>
              </div>
              <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: '#8895A8' }}>
                  <span style={{ display: 'inline-block', width: 12, height: 3, background: '#0B6E4F', borderRadius: 2 }} />Инциденты
                </span>
                <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: '#8895A8' }}>
                  <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', background: '#8B1E2F' }} />Пики
                </span>
              </div>
            </div>
            <MismatchChart data={chartData} />
          </div>
        </div>

        <div className={`tab-p ${tab === 'speedometer' ? 'on' : ''}`}>
          <div className="ch-panel" style={{ textAlign: 'center', padding: '40px 20px' }}>
            <div className="ch-hd" style={{ textAlign: 'center', marginBottom: '40px' }}>
              <div>
                <div className="ch-title">Спидометр Matcher</div>
                <div className="ch-meta">Производительность ядра vs Задержка системы</div>
              </div>
            </div>
            <MatcherSpeedometer 
              avgLatency={speedometer.avg_latency_ms || 0} 
              logicLatency={speedometer.avg_logic_latency_ms || 0}
              successRate={speedometer.success_rate || 0} 
            />
            <div style={{ marginTop: '30px', display: 'flex', gap: '20px', justifyContent: 'center', flexWrap: 'wrap' }}>
              <div style={{ padding: '15px', background: 'rgba(11,110,79,0.1)', borderRadius: '8px', minWidth: '150px' }}>
                <div style={{ fontSize: '12px', color: '#8895A8', marginBottom: '5px' }}>Core Latency (P50)</div>
                <div style={{ fontSize: '20px', fontWeight: 'bold', color: '#0B6E4F' }}>{(speedometer.p50_logic_latency_ms || 0).toFixed(3)}ms</div>
              </div>
              <div style={{ padding: '15px', background: 'rgba(11,110,79,0.1)', borderRadius: '8px', minWidth: '150px' }}>
                <div style={{ fontSize: '12px', color: '#8895A8', marginBottom: '5px' }}>System Latency (Avg)</div>
                <div style={{ fontSize: '20px', fontWeight: 'bold', color: '#0B6E4F' }}>{(speedometer.avg_latency_ms || 0).toFixed(3)}ms</div>
              </div>
              <div style={{ padding: '15px', background: 'rgba(139,30,47,0.1)', borderRadius: '8px', minWidth: '150px' }}>
                <div style={{ fontSize: '12px', color: '#8895A8', marginBottom: '5px' }}>Ошибок (Total)</div>
                <div style={{ fontSize: '20px', fontWeight: 'bold', color: '#8B1E2F' }}>{speedometer.failed_matches || 0}</div>
              </div>
            </div>
            <p style={{ marginTop: '20px', fontSize: '11px', color: '#8895A8', maxWidth: '500px', margin: '20px auto' }}>
              * Core Latency показывает чистое время работы алгоритма в памяти. <br/>
              * System Latency включает время записи в БД и сетевые задержки.
            </p>
          </div>
        </div>

        <div className={`tab-p ${tab === 'table' ? 'on' : ''}`}>
          <div className="tbl-panel">
            <table>
              <thead>
                <tr>
                  <th>Transaction ID</th>
                  <th>Мерчант</th>
                  <th>Тип</th>
                  <th>Задержка</th>
                  <th>Создан</th>
                  <th>Статус</th>
                </tr>
              </thead>
              <tbody>
                {transactions.length === 0 ? (
                  <tr className="empty-row"><td colSpan={6}>Нет данных — ожидание транзакций...</td></tr>
                ) : transactions.map(tx => (
                  <tr key={tx.transaction_id}>
                    <td className="tx">{tx.transaction_id.slice(0, 18)}…</td>
                    <td>{tx.merchant_id || '—'}</td>
                    <td style={{ color: '#8895A8', fontSize: 13 }}>{tx.tx_type}</td>
                    <td style={{ color: '#0B6E4F', fontWeight: 'bold' }}>{(tx.processing_time_ms || 0).toFixed(3)} мс</td>
                    <td style={{ color: '#8895A8', fontSize: 13 }}>{fmtAge(tx.created_at)}</td>
                    <td><StatusBadge status={tx.overall_status} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className={`tab-p ${tab === 'incident' ? 'on' : ''}`}>
          <div className="inc-panel">
            {!activeIncident ? (
              <div className="inc-no">
                <span className="inc-badge none" style={{ marginBottom: 16, display: 'inline-flex' }}>Нет открытых инцидентов</span>
                <p style={{ marginTop: 16 }}>Все транзакции сходятся корректно.</p>
              </div>
            ) : (
              <>
                <div className="inc-top">
                  <span className="inc-badge">Инцидент #{activeIncident.id} · severity {activeIncident.severity}</span>
                  <span className="inc-time">Обнаружен {fmtAge(activeIncident.created_at)} · {activeIncident.transaction_id.slice(0, 13)}…</span>
                </div>
                <h3 className="inc-h">{incidentTypeLabel(activeIncident.incident_type)}</h3>
                <p className="inc-sub">{activeIncident.description}</p>
                <div className="inc-grid">
                  <div className="inc-f">
                    <div className="inc-fl">Тип</div>
                    <div className="inc-fv mis">{activeIncident.incident_type}</div>
                  </div>
                  <div className="inc-f">
                    <div className="inc-fl">Транзакция</div>
                    <div className="inc-fv" style={{ fontSize: 13, wordBreak: 'break-all' }}>{activeIncident.transaction_id}</div>
                  </div>
                  <div className="inc-f">
                    <div className="inc-fl">Статус</div>
                    <div className={`inc-fv ${activeIncident.status === 'open' ? 'mis' : 'ok'}`}>{activeIncident.status}</div>
                  </div>
                  <div className="inc-f">
                    <div className="inc-fl">Severity</div>
                    <div className={`inc-fv ${activeIncident.severity >= 3 ? 'mis' : ''}`}>{severityLabel(activeIncident.severity)}</div>
                  </div>
                </div>
                <div className="inc-desc">{activeIncident.description}</div>
                {incidents.length > 1 && (
                  <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
                    {incidents.map((inc, i) => (
                      <button key={inc.id} onClick={() => setActiveIncident(inc)}
                        style={{ padding: '4px 12px', borderRadius: 8, border: '1px solid rgba(255,255,255,.1)', background: activeIncident.id === inc.id ? 'rgba(139,30,47,.3)' : 'rgba(255,255,255,.06)', color: '#C9D1D9', cursor: 'pointer', fontSize: 12 }}>
                        #{inc.id}
                      </button>
                    ))}
                  </div>
                )}
                <div className="inc-acts">
                  <button className="btn-confirm" disabled={actionLoading || activeIncident.status !== 'open'} onClick={handleAck}>✓ Подтвердить</button>
                  <button className="btn-close-i" disabled={actionLoading} onClick={handleResolve}>Закрыть инцидент</button>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}

function incidentTypeLabel(t) {
  const map = {
    amount_mismatch: 'Расхождение суммы',
    fee_mismatch: 'Расхождение комиссии',
    missing_source: 'Отсутствует источник',
    duplicate: 'Дубликат транзакции',
    timeout: 'Таймаут сверки',
  }
  return map[t] || t
}

function severityLabel(s) {
  if (s >= 4) return 'CRITICAL'
  if (s >= 3) return 'HIGH'
  if (s >= 2) return 'MEDIUM'
  return 'LOW'
}

function HeroArt() {
  return (
    <div className="hero-art">
      <svg className="hero-svg" viewBox="0 0 560 520" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="gp" x="0" y="0" width="90" height="90" patternUnits="userSpaceOnUse">
            <g transform="translate(45,45)">
              <rect x="-28" y="-28" width="56" height="56" rx="3" fill="none" stroke="#0B6E4F" strokeWidth=".7" opacity=".35"/>
              <rect x="-28" y="-28" width="56" height="56" rx="3" fill="none" stroke="#0B6E4F" strokeWidth=".7" opacity=".35" transform="rotate(45)"/>
              <polygon points="0,-18 12.7,-12.7 18,0 12.7,12.7 0,18 -12.7,12.7 -18,0 -12.7,-12.7" fill="none" stroke="#C9A13B" strokeWidth=".5" opacity=".3"/>
              <circle r="4" fill="none" stroke="#0B6E4F" strokeWidth=".5" opacity=".25"/>
            </g>
          </pattern>
          <radialGradient id="bg-grad" cx="50%" cy="50%" r="60%">
            <stop offset="0%" stopColor="#FFF3E0" stopOpacity="1"/>
            <stop offset="100%" stopColor="#FFE8C8" stopOpacity="1"/>
          </radialGradient>
          <filter id="glow"><feGaussianBlur stdDeviation="4" result="blur"/><feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
          <filter id="glow-r"><feGaussianBlur stdDeviation="6" result="blur"/><feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
          <path id="path1" d="M 110 280 C 180 280 200 210 280 210"/>
        </defs>
        <rect width="560" height="520" rx="24" fill="url(#bg-grad)"/>
        <rect width="560" height="520" rx="24" fill="url(#gp)" opacity=".9"/>
        <g className="r1" transform="translate(280,260)">
          <polygon points="0,-130 18.4,-18.4 130,0 18.4,18.4 0,130 -18.4,18.4 -130,0 -18.4,-18.4" fill="none" stroke="#0B6E4F" strokeWidth="1" opacity=".15"/>
        </g>
        <g className="r2" transform="translate(280,260)">
          <polygon points="0,-90 12.7,-12.7 90,0 12.7,12.7 0,90 -12.7,12.7 -90,0 -12.7,-12.7" fill="none" stroke="#C9A13B" strokeWidth="1" opacity=".22"/>
        </g>
        <g className="r3" transform="translate(280,260)">
          <polygon points="0,-165 23.3,-23.3 165,0 23.3,23.3 0,165 -23.3,23.3 -165,0 -23.3,-23.3" fill="none" stroke="#8B1E2F" strokeWidth=".7" opacity=".1"/>
        </g>
        <line x1="148" y1="280" x2="242" y2="210" stroke="#0B6E4F" strokeWidth="1.8" className="dl" opacity=".7"/>
        <line x1="318" y1="210" x2="412" y2="280" stroke="#0B6E4F" strokeWidth="1.8" className="dl2" opacity=".4"/>
        <line x1="318" y1="210" x2="412" y2="280" stroke="#8B1E2F" strokeWidth="1.8" strokeDasharray="10 6" opacity=".55"/>
        <circle r="5" fill="#0B6E4F" filter="url(#glow)">
          <animateMotion dur="2.2s" repeatCount="indefinite" rotate="auto"><mpath href="#path1"/></animateMotion>
        </circle>
        <g transform="translate(110,280)">
          <circle r="56" fill="white" opacity=".92" stroke="#0B6E4F" strokeWidth="2"/>
          <circle r="46" fill="none" stroke="#0B6E4F" strokeWidth=".8" opacity=".4"/>
          <text textAnchor="middle" y="-8" fontFamily="'Playfair Display',serif" fontSize="14" fontWeight="700" fill="#0B6E4F">Payme</text>
          <text textAnchor="middle" y="10" fontFamily="Inter,sans-serif" fontSize="11" fill="#6B6B80">Мерчант</text>
          <text textAnchor="middle" y="82" fontFamily="Inter,sans-serif" fontSize="12" fontWeight="600" fill="#1A1A2E">ООО «Алмаз»</text>
        </g>
        <g transform="translate(280,210)">
          <circle r="56" fill="white" opacity=".92" stroke="#0B6E4F" strokeWidth="2"/>
          <circle r="46" fill="none" stroke="#0B6E4F" strokeWidth=".8" opacity=".4"/>
          <text textAnchor="middle" y="-8" fontFamily="'Playfair Display',serif" fontSize="14" fontWeight="700" fill="#0B6E4F">Click</text>
          <text textAnchor="middle" y="10" fontFamily="Inter,sans-serif" fontSize="11" fill="#6B6B80">Шлюз</text>
          <text textAnchor="middle" y="82" fontFamily="Inter,sans-serif" fontSize="12" fontWeight="600" fill="#1A1A2E">2.1% комиссия</text>
        </g>
        <g transform="translate(450,280)">
          <circle r="66" fill="none" stroke="#8B1E2F" strokeWidth="1.5" opacity=".4"/>
          <circle r="56" fill="white" opacity=".92" stroke="#8B1E2F" strokeWidth="2"/>
          <circle r="46" fill="none" stroke="#8B1E2F" strokeWidth=".8" opacity=".4"/>
          <text textAnchor="middle" y="-8" fontFamily="'Playfair Display',serif" fontSize="14" fontWeight="700" fill="#8B1E2F">НБУ</text>
          <text textAnchor="middle" y="10" fontFamily="Inter,sans-serif" fontSize="11" fill="#6B6B80">Банк</text>
          <text textAnchor="middle" y="82" fontFamily="Inter,sans-serif" fontSize="12" fontWeight="600" fill="#8B1E2F">1.8% записано</text>
        </g>
        <g transform="translate(415,160)">
          <rect x="-64" y="-18" width="128" height="36" rx="18" fill="#8B1E2F"/>
          <text textAnchor="middle" y="6" fontFamily="Inter,sans-serif" fontSize="12" fontWeight="700" fill="white" letterSpacing="0.5">⚠ fee_mismatch</text>
        </g>
        <line x1="415" y1="142" x2="415" y2="158" stroke="#8B1E2F" strokeWidth="1" strokeDasharray="3,3" opacity=".6"/>
        <rect x="20" y="440" width="200" height="64" rx="14" fill="white" opacity=".88" stroke="#C9A13B" strokeWidth="1"/>
        <circle cx="42" cy="472" r="6" fill="#2ECC71"/>
        <text x="56" y="466" fontFamily="Inter,sans-serif" fontSize="11" fontWeight="600" fill="#6B6B80">Задержка обработки</text>
        <text x="56" y="484" fontFamily="'Playfair Display',serif" fontSize="20" fontWeight="700" fill="#0B6E4F">1.859 мс</text>
        <rect x="340" y="30" width="200" height="64" rx="14" fill="white" opacity=".88" stroke="#C9A13B" strokeWidth="1"/>
        <circle cx="362" cy="62" r="6" fill="#0B6E4F"/>
        <text x="376" y="56" fontFamily="Inter,sans-serif" fontSize="11" fontWeight="600" fill="#6B6B80">Событий в секунду</text>
        <text x="376" y="74" fontFamily="'Playfair Display',serif" fontSize="20" fontWeight="700" fill="#0B6E4F">&gt; 1000</text>
      </svg>
    </div>
  )
}

function OrnDivider1() {
  return (
    <div className="orn-sep">
      <svg viewBox="0 0 1440 52" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="od1" x="0" y="0" width="72" height="52" patternUnits="userSpaceOnUse">
            <g transform="translate(36,26)">
              <polygon points="0,-20 5.4,-5.4 20,0 5.4,5.4 0,20 -5.4,5.4 -20,0 -5.4,-5.4" fill="none" stroke="#C9A13B" strokeWidth=".9" opacity=".45"/>
              <polygon points="0,-12 3.4,-3.4 12,0 3.4,3.4 0,12 -3.4,3.4 -12,0 -3.4,-3.4" fill="none" stroke="#0B6E4F" strokeWidth=".7" opacity=".35"/>
              <circle r="3" fill="#C9A13B" opacity=".4"/>
              <line x1="-36" y1="0" x2="-20" y2="0" stroke="#C9A13B" strokeWidth=".6" opacity=".3"/>
              <line x1="20" y1="0" x2="36" y2="0" stroke="#C9A13B" strokeWidth=".6" opacity=".3"/>
            </g>
          </pattern>
        </defs>
        <rect width="1440" height="52" fill="url(#od1)"/>
      </svg>
    </div>
  )
}

function OrnDivider2() {
  return (
    <div className="orn-sep">
      <svg viewBox="0 0 1440 52" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="od2" x="0" y="0" width="60" height="52" patternUnits="userSpaceOnUse">
            <g transform="translate(30,26)">
              <circle r="20" fill="none" stroke="#0B6E4F" strokeWidth=".8" opacity=".3"/>
              <circle r="14" fill="none" stroke="#C9A13B" strokeWidth=".6" opacity=".25"/>
              <circle r="4" fill="#0B6E4F" opacity=".4"/>
              <line x1="-30" y1="0" x2="-20" y2="0" stroke="#0B6E4F" strokeWidth=".6" opacity=".25"/>
              <line x1="20" y1="0" x2="30" y2="0" stroke="#0B6E4F" strokeWidth=".6" opacity=".25"/>
            </g>
          </pattern>
        </defs>
        <rect width="1440" height="52" fill="url(#od2)"/>
      </svg>
    </div>
  )
}

function OrnDivider3() {
  return (
    <div className="orn-sep" style={{ background: '#111420' }}>
      <svg viewBox="0 0 1440 52" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="od3" x="0" y="0" width="56" height="52" patternUnits="userSpaceOnUse">
            <g transform="translate(28,26)">
              <rect x="-18" y="-18" width="36" height="36" fill="none" stroke="#C9A13B" strokeWidth=".7" opacity=".3" transform="rotate(45)"/>
              <rect x="-11" y="-11" width="22" height="22" fill="none" stroke="#0B6E4F" strokeWidth=".6" opacity=".25" transform="rotate(45)"/>
              <circle r="3" fill="#C9A13B" opacity=".35"/>
            </g>
          </pattern>
        </defs>
        <rect width="1440" height="52" fill="url(#od3)"/>
      </svg>
    </div>
  )
}

export default function App() {
  const [theme, setTheme] = useState('light')
  const [lang, setLang] = useState('ru')

  useFadeUp()

  function toggleTheme() {
    const next = theme === 'light' ? 'dark' : 'light'
    setTheme(next)
    document.documentElement.setAttribute('data-theme', next)
  }

  function scrollToDemo() {
    document.getElementById('demo')?.scrollIntoView({ behavior: 'smooth' })
  }

  return (
    <>
      <nav className="nav">
        <div className="wrap">
          <a href="#" className="logo">Chinor</a>
          <div className="nav-links">
            <span className="nav-a" onClick={() => document.getElementById('stats')?.scrollIntoView({ behavior: 'smooth' })}>Потери бизнеса</span>
            <span className="nav-a" onClick={() => document.getElementById('tech')?.scrollIntoView({ behavior: 'smooth' })}>Технология</span>
            <span className="nav-a" onClick={scrollToDemo}>Демо</span>
          </div>
          <div className="nav-right">
            <div className="lang-box">
              <button className={`lang-b ${lang === 'ru' ? 'on' : ''}`} onClick={() => setLang('ru')}>RU</button>
              <button className={`lang-b ${lang === 'uz' ? 'on' : ''}`} onClick={() => setLang('uz')}>UZ</button>
            </div>
            <button className="theme-b" onClick={toggleTheme} title="Тема">{theme === 'light' ? '☀' : '🌙'}</button>
            <button className="btn-nav" onClick={scrollToDemo}>Попробовать демо</button>
          </div>
        </div>
      </nav>

      <section className="hero">
        <div className="wrap" style={{ width: '100%' }}>
          <div className="hero-grid">
            <div>
              <div className="hero-tag fade-up">Финтех-платформа · Узбекистан</div>
              <h1 className="hero-h1 fade-up d1">
                Сверка платежей<br />в <em>реальном времени</em>
              </h1>
              {lang === 'ru' && <p className="hero-h1-uz fade-up d2"></p>}
              <p className="hero-sub fade-up d2">
                {lang === 'ru'
                  ? 'Обнаруживайте расхождения за миллисекунды, а не за часы.  '
                  : 'Farqlarni soatlar emas, millisekundlarda aniqlang. Yashirin komissiyalarsiz.'}
              </p>
              <div className="hero-btns fade-up d3">
                <button className="btn-p" onClick={scrollToDemo}>
                  <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                    <polygon points="5,3 13,8 5,13" fill="currentColor" stroke="none"/>
                  </svg>
                  {lang === 'ru' ? 'Попробовать демо' : 'Demo ko\'rish'}
                </button>
                <button className="btn-g">Документация →</button>
              </div>
            </div>
            <HeroArt />
          </div>
        </div>
      </section>

      <OrnDivider1 />

      <section className="stats-sec" id="stats">
        <div className="wrap">
          <div className="sec-hd">
            <div className="sec-tag fade-up">Аналитика потерь</div>
            <h2 className="sec-h2 fade-up d1">Скрытые потери вашего бизнеса</h2>
            <p className="sec-sub fade-up d2">Ежедневно миллионы сумов исчезают незаметно в зазорах между мерчантом, платёжным шлюзом и банком.</p>
          </div>
          <div className="stats-grid">
            {[
              { num: '3–4%', title: 'Сбои платежей', desc: 'цифровых платежей завершаются неудачей, и больше половины таких сбоев никогда не превращаются в успешную оплату.' },
              { num: '5–20ч', title: 'Потери времени', desc: 'Более 70% компаний тратят 5–20 часов в неделю на ручную сверку и разбор расхождений.' },
              { num: 'Loss', title: 'Утечка выручки', desc: 'Даже небольшая доля ошибок в платёжном потоке оборачивается постоянной утечкой выручки и потерей продуктивности.' },
            ].map((s, i) => (
              <div className={`sc fade-up d${i + 1}`} key={i}>
                <div className="sc-num">{s.num}</div>
                <div className="sc-title">{s.title}</div>
                <p className="sc-desc">{s.desc}</p>
                <div className="gold-bar" />
              </div>
            ))}
          </div>
          <div className="stats-note fade-up">
            Распределённые платёжные системы страдают от <strong>позднего обнаружения расхождений</strong>,{' '}
            <strong>пропущенных транзакций</strong> и <strong>дублей</strong>. Chinor закрывает этот разрыв
            автоматически — в режиме реального времени.
          </div>
        </div>
      </section>

      <OrnDivider2 />

      <section className="tech-sec" id="tech">
        <div className="wrap">
          <div className="sec-hd">
            <div className="sec-tag fade-up">Архитектура</div>
            <h2 className="sec-h2 fade-up d1">Технология, которой можно доверять</h2>
            <p className="sec-sub fade-up d2">Построена на Rust и in-memory обработке.  Мы ценим производительность.</p>
          </div>
          <div className="metrics-row fade-up">
            {[
              { num: '< 2мс', lbl: 'Полный цикл обработки события' },
              { num: '1000/сек', lbl: 'Пропускная способность' },
              { num: '99.99%', lbl: 'Точность обнаружения инцидентов' },
            ].map((m, i) => (
              <div className="met" key={i}>
                <div className="met-num" style={{ fontSize: m.num.length > 6 ? '48px' : '72px' }}>{m.num}</div>
                <div className="met-lbl">{m.lbl}</div>
              </div>
            ))}
          </div>
          <div className="feats">
            {[
              {
                title: 'Сверка в реальном времени',
                desc: 'Rust + in-memory processing. Каждая транзакция сверяется мгновенно без задержек на запись в базу.',
                icon: <polyline points="2,12 8,6 14,11 20,4"/>,
              },
              {
                title: 'Автоматическое обнаружение расхождений',
                desc: 'Сверка всех сумм, комиссий, валют и статусов  между всеми пользователями цепи.',
                icon: <><circle cx="11" cy="11" r="9"/><path d="M11 6 L11 11 L15 14"/></>,
              },
              {
                title: 'Эскалация критических инцидентов',
                desc: 'Критические инциденты выводятся оператору меньше чем за 10 секунд.',
                icon: <><path d="M11 2L3 7v8l8 5 8-5V7L11 2z"/><polyline points="11,12 11,7"/><circle cx="11" cy="14.5" r=".5" fill="currentColor"/></>,
              },
              {
                title: 'Безопасность и идемпотентность',
                desc: 'HMAC-подпись каждого события, идемпотентная обработка дублей.',
                icon: <><rect x="3" y="11" width="16" height="9" rx="2"/><path d="M7 11V7a4 4 0 0 1 8 0v4"/></>,
              },
            ].map((f, i) => (
              <div className={`feat fade-up d${i + 1}`} key={i}>
                <div className="feat-ico">
                  <svg viewBox="0 0 22 22">{f.icon}</svg>
                </div>
                <div>
                  <h4>{f.title}</h4>
                  <p>{f.desc}</p>
                </div>
              </div>
            ))}
          </div>
          <div className="tech-cta fade-up">
            <button className="btn-p" onClick={scrollToDemo}>Попробовать демо &nbsp;→</button>
          </div>
        </div>
      </section>

      <OrnDivider3 />

      <DemoSection />

      <footer>
        <div className="wrap">
          <div className="f-gold-line" />
          <div className="f-inner">
            <div>
              <div className="f-logo">Chinor</div>
              <div className="f-copy">© 2026 Chinor Uzbekistan.<br />Сверка платежей в реальном времени.</div>
            </div>
            <div className="f-links">
              <span className="f-a">Политика конфиденциальности</span>
              <span className="f-a">Условия использования</span>
              <span className="f-a">Безопасность</span>
              <span className="f-a">Связаться с нами</span>
            </div>
          </div>
        </div>
      </footer>
    </>
  )
}
