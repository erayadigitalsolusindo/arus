import type { Messages } from '../../types.ts';

const auth: Messages['auth'] = {
  login: {
    docTitle: 'ログイン | ACIRABA',
    titleLine1: 'バックオフィスへアクセス',
    titleLine2: 'ARUS（Aciraba Upgrade System）',
    subtitle: 'ワークスペースを続行するにはログインしてください',
    orEmail: 'またはメールでログイン',
    email: 'メールアドレス',
    emailPlaceholder: 'you@company.com',
    password: 'パスワード',
    forgot: 'パスワードをお忘れですか?',
    showPassword: 'パスワードを表示',
    hidePassword: 'パスワードを非表示',
    remember: 'ログイン状態を保持',
    submit: 'ログイン',
    noAccount: 'アカウントがありませんか?',
    createOne: 'アカウントを作成',
    downloadApk: 'レジAPKをダウンロード',
    emailRequired: 'メールアドレスは必須です。',
    emailInvalid: 'メールアドレスの形式が無効です。',
    passwordRequired: 'パスワードは必須です。'
  },
  register: {
    docTitle: '登録 | ACIRABA',
    title: 'ARUSアカウントを作成',
    subtitle: 'ビジネスを登録して数分で開始',
    businessName: 'ビジネス名',
    businessNamePlaceholder: 'マジュ・ジャヤ店',
    ownerName: 'オーナー名',
    ownerNamePlaceholder: 'ブディ・サントソ',
    outletName: '最初の店舗名',
    outletNamePlaceholder: '本店',
    email: 'メールアドレス',
    phone: '電話番号',
    phonePlaceholder: '0812 3456 7890',
    password: 'パスワード',
    passwordPlaceholder: '最低10文字、英字と数字',
    confirmPassword: 'パスワード（確認）',
    mismatch: 'パスワードが一致しません。',
    strength: { weak: '弱い', fair: '普通', strong: '強い' },
    submit: 'アカウントを作成',
    haveAccount: 'すでにアカウントをお持ちですか?',
    signIn: 'ログイン',
    headline: 'ARUSでビジネス運営を始めましょう。',
    step1: 'オーナーアカウントと最初の店舗が自動的に作成されます',
    step2: 'ExcelまたはCSVから商品をインポート',
    step3: '期首の在庫、売掛金、買掛金を入力',
    step4: 'レジで販売を開始',
    stepsTitle: '登録後に起こること'
  },
  promo: {
    headline: '起業家が雇用を創出!',
    point1: '次のレベルでビジネス機会を最大化する、最も完全なエコシステムと統合。',
    point2: '私たちのアプリがビジネスの成長を支援します。',
    point3: '多機能で使いやすく、正確なデータを提供するPoint of Saleで、より良いビジネス戦略を実現します。',
    point4: '独自のブランドを構築できるホワイトラベル。',
    point5: 'Acirabaは中小企業の発展を支援します。',
    scanRead: '読み取り完了',
    scanTagline: 'スキャンして即販売',
    salesToday: '今日の売上',
    transactions: { one: '{count} 件の取引', other: '{count} 件の取引' }
  }
};

export default auth;
