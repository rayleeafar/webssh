import './globals.css'
import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'WebSSH Manager',
  description: 'SSH + SFTP in your browser — Neon Noir edition',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  )
}
