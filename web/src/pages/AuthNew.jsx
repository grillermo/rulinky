import { useJSONForm } from '../api.js'

export default function AuthNew() {
  const { data, setData, submit, processing, errors } = useJSONForm({ email: '' })

  function onSubmit(e) {
    e.preventDefault()
    submit('POST', '/auth')
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <form onSubmit={onSubmit} className="w-full max-w-sm space-y-4">
        <h1 className="text-xl font-semibold">Sign in to Rulinky</h1>
        <input
          type="email"
          autoComplete="email"
          value={data.email}
          onChange={e => setData('email', e.target.value)}
          placeholder="you@example.com"
          className="w-full border rounded px-3 py-2"
          autoFocus
        />
        {errors.email && <p role="alert" className="text-red-600 text-sm">{errors.email}</p>}
        {errors.base && <p role="alert" className="text-red-600 text-sm">{errors.base}</p>}
        <button
          type="submit"
          disabled={processing}
          className="w-full bg-black text-white rounded px-3 py-2 disabled:opacity-50"
        >
          Send code
        </button>
      </form>
    </div>
  )
}
