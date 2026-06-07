import { useForm } from '@inertiajs/react'

export default function AuthNew() {
  const { data, setData, post, processing, errors } = useForm({ email: '' })

  function submit(e) {
    e.preventDefault()
    post('/auth')
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-4">
        <h1 className="text-xl font-semibold">Sign in to Rulinky</h1>
        <input
          type="email"
          value={data.email}
          onChange={e => setData('email', e.target.value)}
          placeholder="you@example.com"
          className="w-full border rounded px-3 py-2"
          autoFocus
        />
        {errors.email && <p className="text-red-600 text-sm">{errors.email}</p>}
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
