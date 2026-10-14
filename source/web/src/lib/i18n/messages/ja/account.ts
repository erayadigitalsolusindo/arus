import type { Messages } from '../../types.ts';

const account: Messages['account'] = {
  verifyBanner: {
    text: 'メールアドレスがまだ認証されていません。受信トレイ内の認証リンクをご確認ください。',
    resend: 'メールを再送',
    sending: '送信中…',
    sent: '認証メールを送信しました。受信トレイ（および迷惑メールフォルダ）をご確認ください。'
  },
  terms: {
    accept: '同意します',
    termsLink: '利用規約',
    and: 'および',
    privacyLink: 'プライバシーポリシー',
    required: '利用規約とプライバシーポリシーに同意する必要があります。'
  },
  forgot: {
    docTitle: 'パスワードをお忘れの場合 | ACIRABA',
    title: 'パスワードをお忘れですか？',
    subtitle: 'アカウントのメールアドレスを入力してください。新しいパスワードを作成するためのリンクをお送りします。',
    email: 'メールアドレス',
    emailPlaceholder: 'you@company.com',
    submit: 'リセットリンクを送信',
    sending: '送信中…',
    sentTitle: 'メールをご確認ください',
    sent: 'そのメールアドレスが登録されている場合、パスワードをリセットするためのリンクを送信しました。リンクは30分間有効で、1回のみ使用できます。',
    back: 'サインインページに戻る'
  },
  reset: {
    docTitle: 'パスワードリセット | ACIRABA',
    title: '新しいパスワードを作成',
    subtitle: 'アカウントの新しいパスワードを選択してください。すべての端末がサインアウトされます。',
    password: '新しいパスワード',
    confirm: 'パスワードを再入力',
    mismatch: 'パスワードが一致しません。',
    submit: 'パスワードを保存',
    saving: '保存中…',
    successTitle: 'パスワードを変更しました',
    success: '新しいパスワードでサインインしてください。',
    missing: 'リンクが不完全です。メール内のリンクを開くか、新しいリンクをリクエストしてください。',
    requestNew: '新しいリンクをリクエスト',
    login: 'サインインページへ'
  },
  verify: {
    docTitle: 'メール認証 | ACIRABA',
    working: 'メールアドレスを認証中…',
    successTitle: 'メール認証済み',
    success: 'ありがとうございます。メールアドレスが認証されました。',
    failedTitle: '無効なリンク',
    failed: '認証リンクが無効または期限切れです。サインインして、上部のバナーから新しい認証メールをリクエストしてください。',
    toDashboard: 'ダッシュボードへ',
    toLogin: 'サインインページへ'
  }
};
export default account;
