import type { Messages } from '../../types.ts';

const profile: Messages['profile'] = {
  docTitle: 'マイプロフィール | ACIRABA',
  title: 'マイプロフィール',
  loadFailed: 'プロフィールを読み込めませんでした。',
  retry: '再試行',
  banner: { change: 'バナーを変更', add: 'バナーを追加', remove: 'バナーを削除', alt: 'プロフィールバナー' },
  avatar: { change: '写真を変更', add: '写真を追加', remove: '写真を削除', alt: '{name} のプロフィール写真' },
  image: {
    uploading: 'アップロード中…',
    tooBig: '画像は 10 MB 以下にしてください。',
    badType: 'JPG、PNG、WebP の画像を選択してください。',
    hint: 'JPG、PNG、WebP（最大 10 MB）。',
    bannerHint: 'バナーは横長の画像（約 4:1）がきれいに表示されます。'
  },
  outlets: { one: '{count} 店舗', other: '{count} 店舗' },
  verified: 'メール認証済み',
  unverified: 'メール未認証',
  memberSince: '{date} に参加',
  lastLogin: '最終ログイン {date}',
  noLogin: 'ログイン履歴なし',
  about: {
    title: '自己紹介',
    empty: '自己紹介はまだありません。役割について簡単に書きましょう。',
    addBio: '自己紹介を書く',
    contact: '連絡先'
  },
  completeness: {
    title: 'プロフィールの完成度',
    done: 'プロフィールは完成しています。',
    next: '次のステップ',
    percent: '{value}%',
    step: {
      avatar: 'プロフィール写真を追加',
      phone: '電話番号を入力',
      job_title: '役職を入力',
      location: '所在地を入力',
      bio: '自己紹介を書く',
      banner: 'バナーを設定',
      email_verified: 'メールを認証',
      mfa: '2 段階認証を有効にする'
    }
  },
  tabs: { label: 'プロフィールのセクション', overview: '概要', info: '情報', security: 'セキュリティ', prefs: '設定' },
  info: {
    title: '個人情報',
    subtitle: 'プロフィールと操作履歴に表示されます。',
    name: '氏名',
    namePh: 'お名前',
    email: 'メール',
    emailHint: 'メールはログインに使用され、ここでは変更できません。',
    phone: '電話番号',
    phonePh: '0812 3456 7890',
    phoneHint: '先頭の 0 は自動的に +62 になります。',
    jobTitle: '役職',
    jobTitlePh: '例：店長',
    location: '所在地',
    locationPh: '例：バンドン',
    bio: '自己紹介',
    bioPh: 'ご自身や役割について簡単に書いてください…',
    bioHint: 'GitHub 風マークダウン：**太字**、*斜体*、~~取り消し線~~、`コード`、リスト、- [ ] タスク、引用、表、リンク。',
    save: '変更を保存',
    saving: '保存中…',
    saved: 'プロフィールを保存しました。',
    discard: '破棄',
    dirty: '未保存の変更があります'
  },
  security: {
    title: 'パスワードの変更',
    subtitle: '他のサービスで使っていない固有のパスワードを使用してください。',
    current: '現在のパスワード',
    next: '新しいパスワード',
    confirm: '新しいパスワード（確認）',
    show: 'パスワードを表示',
    hide: 'パスワードを隠す',
    hint: '10 文字以上、英字と数字を含めてください。',
    mismatch: 'パスワードが一致しません。',
    submit: 'パスワードを変更',
    saving: '保存中…',
    logoutNote: '変更後はすべての端末からログアウトされ、再ログインが必要です。',
    strength: { label: '強度', weak: '弱い', good: '良い', strong: '強い' },
    accounts: {
      title: 'アカウントとアクセス',
      google: 'Google',
      googleLinked: '連携済み',
      googleNotLinked: '未連携',
      googleNote: '連携した Google アカウントでログインできます。',
      pin: '承認者 PIN',
      pinNote: 'レジでの重要な操作を承認する PIN です。',
      pinManage: 'PIN を管理'
    }
  },
  stats: { actions: '30 日間の操作', logins: '30 日間のログイン', outlets: '店舗', tenure: '参加期間', days: '{count} 日' },
  overview: {
    access: { title: 'アクセスと役割', role: '役割', all: 'すべてのモジュールにフルアクセス', modules: '{count} 個のモジュールを利用可能', posOnly: 'レジ専用アカウント', locked: '{count} 個のモジュールがプランでロック中' },
    outlet: { title: '現在の店舗', current: 'このセッション', total: 'アクセス可能：{count}' },
    activity: {
      title: '最近の操作',
      subtitle: 'ARUS でのご自身の操作履歴です。',
      empty: '記録された操作はまだありません。',
      more: 'さらに読み込む',
      loading: '読み込み中…',
      failed: '操作履歴を読み込めませんでした。',
      from: '{ip} から',
      actions: {
        user_profile: 'プロフィールを更新',
        user_profile_image: 'プロフィール写真/バナーを変更',
        user_password_change: 'パスワードを変更',
        user_mfa_enable: '2 段階認証を有効化',
        user_mfa_disable: '2 段階認証を無効化',
        user_mfa_recovery: 'リカバリーコードを再生成',
        user_mfa_reset: 'ユーザーの 2 段階認証をリセット',
        user_pin_set: '承認者 PIN を設定'
      }
    }
  },
  mfa: {
    title: '2 段階認証（2FA）',
    subtitle: 'パスワードに加えて、認証アプリの 6 桁コードをログイン時に要求します。',
    on: '有効',
    off: '未設定',
    enable: '2FA を有効にする',
    disable: '2FA を無効にする',
    stepScan: '1. Google Authenticator、Authy、1Password でこの QR コードを読み取ってください。',
    qrAlt: '認証アプリ用 QR コード',
    manual: '読み取れない場合は、このキーを手動で入力してください：',
    stepCode: '2. アプリに表示される 6 桁のコードを入力してください。',
    code: '6 桁のコード',
    verify: '認証して有効化',
    cancel: 'キャンセル',
    recoveryTitle: 'リカバリーコードを保存',
    recoveryNote: 'このコードは一度しか表示されません。認証端末を紛失した場合、各コードを 1 回だけ使用できます。',
    copy: 'すべてコピー',
    copied: 'コピーしました',
    download: '.txt をダウンロード',
    saved: '保存しました',
    remaining: { one: 'リカバリーコード残り {count} 個', other: 'リカバリーコード残り {count} 個' },
    regenerate: 'リカバリーコードを再生成',
    regenerateNote: '古いコードはすぐに無効になります。認証アプリの 6 桁コードを入力してください。',
    disableNote: '2FA を無効にするには、パスワードと 6 桁コード（またはリカバリーコード 1 つ）を入力してください。',
    password: 'パスワード',
    recoveryCodeOrTotp: '6 桁コードまたはリカバリーコード',
    confirmDisable: '無効にする',
    lostDevice: '端末とリカバリーコードを紛失した場合は、オーナー/管理者に 2FA のリセットを依頼してください。',
    loadFailed: '2FA の状態を読み込めませんでした。',
    done: '2FA は有効です。'
  },
  prefs: {
    title: '設定',
    subtitle: 'この端末でのこのアカウントの表示設定です。',
    language: '表示言語',
    languageNote: '言語はこのブラウザに保存され、すべてのページに適用されます。'
  }
};

export default profile;
