'use client'

import dynamic from 'next/dynamic'

// xterm uses the browser-only global `self` at module-evaluation time, so it
// must never be imported during SSR.  dynamic() with ssr:false defers the
// import to the client, where `self` exists.
const TerminalClient = dynamic(() => import('./TerminalClient'), { ssr: false })

export default function TerminalPage() {
  return <TerminalClient />
}
