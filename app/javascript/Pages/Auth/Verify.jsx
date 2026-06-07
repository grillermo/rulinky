import { useForm, Link } from '@inertiajs/react'

export default function AuthVerify({ email }) {
  const { data, setData, post, processing, errors } = useForm({ code: '' })

  function submit(e) {
    e.preventDefault()
    post('/auth/verify')
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-4">
        <h1 className="text-xl font-semibold">Enter your code</h1>
        <p className="text-sm text-gray-600">We sent a 6-digit code to {email}.</p>
        <input
          type="text"
          inputMode="numeric"
          pattern="\d*"
          maxLength={6}
          value={data.code}
          onChange={e => setData('code', e.target.value)}
          placeholder="123456"
          className="w-full border rounded px-3 py-2 tracking-widest text-center"
          autoFocus
        />
        {errors.code && <p className="text-red-600 text-sm">{errors.code}</p>}
        <button
          type="submit"
          disabled={processing}
          className="w-full bg-black text-white rounded px-3 py-2 disabled:opacity-50"
        >
          Verify
        </button>
        <Link href="/auth/new" className="block text-center text-sm text-gray-600 underline">
          Use a different email / resend
        </Link>
      </form>
    </div>
  )
}
