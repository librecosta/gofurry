import { createFileRoute } from '@tanstack/react-router'
import { StartPage } from '../start/page'

// One persistent shell across language URLs: keep the dialog, query and decoded
// wallpaper mounted while the URL-authoritative locale changes.
export const Route = createFileRoute('/_start')({ component: StartPage })
