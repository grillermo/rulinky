import { createRoot } from 'react-dom/client'
import AuthNew from './pages/AuthNew.jsx'
import AuthVerify from './pages/AuthVerify.jsx'
import LinksIndex from './pages/LinksIndex.jsx'

const pages = { 'Auth/New': AuthNew, 'Auth/Verify': AuthVerify, 'Links/Index': LinksIndex }
const page = window.__PAGE__ || { component: 'Auth/New', props: {} }
const Page = pages[page.component] || AuthNew

createRoot(document.getElementById('app')).render(<Page {...(page.props || {})} />)
