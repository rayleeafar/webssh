'use client'

export default function EmptyState() {
  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#050508',
        userSelect: 'none',
      }}
    >
      {/* ASCII terminal art */}
      <pre
        style={{
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: 11,
          color: '#1a1a30',
          lineHeight: 1.4,
          marginBottom: 32,
          textAlign: 'center',
        }}
      >{`
 ╔══════════════════╗
 ║  >_              ║
 ║                  ║
 ║  $ █             ║
 ║                  ║
 ╚══════════════════╝
      `}</pre>

      <div
        style={{
          fontFamily: "'Orbitron', sans-serif",
          fontSize: 13,
          fontWeight: 700,
          color: '#303060',
          letterSpacing: '0.3em',
          marginBottom: 16,
        }}
      >
        SELECT A NODE TO BEGIN
      </div>

      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          color: '#252545',
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: 12,
        }}
      >
        <span>terminal ready</span>
        <span
          style={{
            display: 'inline-block',
            width: 8,
            height: 14,
            background: '#303060',
            animation: 'blink 1s step-end infinite',
          }}
          className="animate-blink"
        />
      </div>

      {/* Subtle grid pattern */}
      <div
        style={{
          position: 'absolute',
          inset: 0,
          backgroundImage:
            'linear-gradient(rgba(0,255,255,0.015) 1px, transparent 1px), linear-gradient(90deg, rgba(0,255,255,0.015) 1px, transparent 1px)',
          backgroundSize: '40px 40px',
          pointerEvents: 'none',
        }}
      />
    </div>
  )
}
