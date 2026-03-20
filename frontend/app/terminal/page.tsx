import { Suspense } from 'react'
import TerminalPage from './TerminalClient'

export default function Page() {
  return (
    <Suspense>
      <TerminalPage />
    </Suspense>
  )
}
