import { Suspense } from 'react'
import SFTPPage from './PageClient'

export default function Page() {
  return (
    <Suspense>
      <SFTPPage />
    </Suspense>
  )
}
