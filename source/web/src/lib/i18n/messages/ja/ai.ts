import type { Messages } from '../../types.ts';

const ai: Messages['ai'] = {
  docTitle: 'HalloArus | ACIRABA',
  title: 'HalloArus',
  subtitle: 'ビジネスデータを読み取り、結論を導き出し、アドバイスを提供するAIアシスタント。',
  readOnly: '読み取り専用',
  readOnlyHint: 'HalloArus はデータの追加・変更・削除を行うことはできません。',
  approvalMode: '変更には承認が必要です',
  approvalModeHint: 'HalloArus はデータ変更を提案するだけです。「承認」を押すまでデータは変わらず、取り消しには承認者のPINも必要です。',
  privacy:
    '集計された数値は、選択したAIプロバイダー（Claude、ChatGPT、Gemini、Qwen、またはカスタムプロバイダー）へ、事業所有のAPIキー（または無料のARUSアカウント）を使用して送信されます。顧客データ（氏名、電話番号、住所）は送信されません。顧客は「Pelanggan 1、2、…」としてのみ表示されます。',
  settings: '設定',
  setup: {
    title: 'AIに接続',
    intro:
      'HalloArus はご自身のビジネスのAPIキーを使用します。利用料金はAIプロバイダーが直接お客様のアカウントに請求します。{console} でキーを作成し、下に貼り付けてください。プロバイダーごとにキーが保存され、アクティブなのは1つのみです。',
    providerLabel: 'AIプロバイダー',
    keyLabel: '{provider} APIキー',
    keyReplaceHint: 'キーを変更しない場合は空欄のままで問題ありません。',
    keySaved: 'キーを保存しました（末尾 {hint}）。暗号化されて保存されるため、再度表示することはできません。',
    keyNone: 'このプロバイダーのキーはまだありません。',
    modelLabel: 'モデル',
    save: '保存して有効化',
    saving: '確認して保存中…',
    saved: '設定を保存しました。',
    removeKey: '{provider} のキーを削除',
    removeConfirm:
      '{provider} のAPIキーを削除しますか？ アクティブなプロバイダーの場合、HalloArus はキーを持つ別のプロバイダーに切り替えます（キーが設定されるまで停止します）。',
    needKey: '先に {provider} のAPIキーを入力してください。',
    notConfigured: 'HalloArus はまだアクティブではありません。事業主にこのページでAI用APIキーの設定を依頼してください。',
    notConfiguredManage: 'HalloArus を有効化するには、AI用APIキー（Claude、ChatGPT、Gemini、Qwen）を設定してください。'
  },
  providers: {
    anthropic: { name: 'Claude (Anthropic)', placeholder: 'sk-ant-…', console: 'console.anthropic.com' },
    openai: { name: 'ChatGPT (OpenAI)', placeholder: 'sk-…', console: 'platform.openai.com' },
    gemini: { name: 'Gemini (Google)', placeholder: 'AIza…', console: 'aistudio.google.com' },
    qwen: { name: 'Qwen (Alibaba)', placeholder: 'sk-…', console: 'modelstudio.console.alibabacloud.com (シンガポールリージョン)' }
  },
  models: {
    'claude-sonnet-5-5': 'Claude Sonnet 5.5（バランス良好、推奨）',
    'claude-opus-5-5': 'Claude Opus 5.5（より深い分析）',
    'claude-fable-5-1': 'Claude Fable 5.1（最大能力）',
    'claude-haiku-5-5': 'Claude Haiku 5.5（高速・低コスト）',
    'gpt-5': 'GPT-5（バランス良好）',
    'gpt-5-mini': 'GPT-5 mini（高速・低コスト）',
    'gpt-5.6-sol': 'GPT-5.6 Sol（最新）',
    'gemini-2.5-pro': 'Gemini 2.5 Pro（より深い分析）',
    'gemini-2.5-flash': 'Gemini 2.5 Flash（高速・低コスト）',
    'gemini-3-flash-preview': 'Gemini 3 Flash（プレビュー）',
    'qwen3-max': 'Qwen3 Max（最大能力）',
    'qwen-plus': 'Qwen Plus（バランス良好）',
    'qwen-flash': 'Qwen Flash（高速・低コスト）'
  },
  policy: {
    disabled: 'このアカウントではプラットフォーム運営者によりHalloArusが無効化されています。有効化が必要な場合はARUS運営者にお問い合わせください。',
    limited: 'プラットフォーム運営者がこのアカウントのHalloArusのデータや機能を一部制限しているため、答えられない質問があります。',
    noHistory: '会話履歴の保存はプラットフォーム運営者により無効化されています。'
  },
  free: {
    banner: '無料アカウント：本日の残り {remaining}／{limit} 件。ご自身のAPIキーを設定すれば無制限に利用できます。',
    exhausted: '本日の無料枠は使い切りました（明日リセットされます）。ご自身のAPIキーを設定すれば制限なく続行できます。',
    useOwn: '自分のキーを使用'
  },
  connector: {
    button: 'AIコネクタ',
    title: 'AIアプリに接続',
    intro:
      'ご自身の Claude、ChatGPT、Gemini、Qwen サブスクリプション（APIキー不要）を利用：ARUS を MCP コネクタとして追加し、そのアプリで直接ビジネスデータを質問できます。コネクタは読み取り専用で、HalloArus と同じ制限に従います。',
    appsTitle: 'アプリを選択',
    appsNote: 'コネクタの対応状況はアプリ・プランによって異なります。利用できない場合は、上の設定にあるAPIキーでの HalloArus チャットをご利用ください。',
    cmdLabel: 'コマンド',
    apps: {
      claude: {
        name: 'Claude',
        s1: '「Settings → Connectors → Add custom connector」を開きます。',
        s2: '名前（例：ARUS）を入力し、上のコネクタアドレスを貼り付けて保存します。',
        s3: '[Connect] をクリックすると、読み取りアクセスを承認するため ARUS に移動します。'
      },
      chatgpt: {
        name: 'ChatGPT',
        s1: '「Settings → Apps & Connectors → Advanced settings」を開き、Developer mode を有効にします（一部のプランのみ）。',
        s2: '「Apps & Connectors → Create」に戻り、名前（例：ARUS）を入力し、上のコネクタアドレスを MCP Server URL として貼り付け、OAuth 認証を選択して Create をクリックします。',
        s3: '新しい会話で + メニュー → Developer mode から ARUS を選択し、ARUS に転送されたら読み取りアクセスを承認します。'
      },
      gemini: {
        name: 'Gemini (Gemini CLI)',
        s1: 'Gemini CLI をインストールし、下のコマンドを実行します。',
        s2: 'gemini を実行し、/mcp auth arus と入力して、開いたブラウザで読み取りアクセスを承認します。',
        s3: 'Gemini CLI で直接ビジネスデータを質問できます。Gemini の Web アプリはまだカスタムコネクタに対応していません。'
      },
      qwen: {
        name: 'Qwen (Qwen Code)',
        s1: 'Qwen Code をインストールし、下のコマンドを実行します。',
        s2: 'qwen を実行し、表示された認可（/mcp）を完了して、ブラウザで読み取りアクセスを承認します。',
        s3: 'Qwen Code で直接ビジネスデータを質問できます。'
      }
    },
    urlLabel: 'コネクタアドレス',
    copy: 'コピー',
    copied: 'コピー済み',
    listTitle: 'アクティブなコネクタ',
    none: '接続されているコネクタはまだありません。',
    by: '{name} による',
    lastUsed: '最終使用 {when}',
    never: '使用されたことはありません',
    revoke: '取り消す',
    revokeConfirm: 'このコネクタのアクセスを取り消しますか？ Claude はこの許可を通じて ARUS データを読み取れなくなります。',
    revoked: 'コネクタのアクセスを取り消しました。'
  },
  connect: {
    docTitle: 'Claude を許可 | ACIRABA',
    title: '{name} が ARUS データを読み取ることを許可しますか？',
    lead: '{user} としてログインしています。{name}（{host} に戻ります）が以下への許可を求めています：',
    c1: 'アクセス権限の範囲で、売上・在庫・売掛金/買掛金・現金・会計のサマリーを読み取ります。',
    c2: '顧客は「Pelanggan 1、2、…」としてのみ表示されます（氏名・電話番号・住所は送信されません）。',
    c3: 'データの追加・変更・削除はできません。',
    allow: '許可',
    deny: '拒否',
    working: 'リダイレクト中…',
    invalid: 'リクエストが無効または期限切れです。Claude アプリからやり直してください。',
    revokeHint: 'アクセスはいつでも HalloArus → Claude コネクタ から取り消せます。'
  },
  empty: {
    title: 'ビジネスデータについて何でも質問できます',
    hint: 'HalloArus はアクセス権限の範囲で売上・在庫・売掛金/買掛金・現金・会計を読み取り、まとめとアドバイスを提供します。'
  },
  suggestions: {
    s1: '直近30日間の売上は、その前の30日間と比べてどうでしたか？',
    s2: '今月、最も売れて利益が出ている商品はどれですか？',
    s3: '在庫が少なくなっていて、すぐに仕入れが必要な商品はどれですか？',
    s4: '回収リスクのある売掛金はありますか？ 最初に請求すべき相手は誰ですか？',
    s5: 'レジで現金の過不足や、多くの取消ノートはありませんか？',
    s6: '動きのない在庫で、セール（処分）すべき商品はどれですか？'
  },
  input: {
    placeholder: '質問を入力…（Enter で送信、Shift+Enter で改行）',
    send: '送信',
    clear: '新しい会話',
    stop: 'キャンセル'
  },
  thinking: 'データを読み取り、分析中…',
  you: 'あなた',
  assistant: 'HalloArus',
  dataRead: '読み取ったデータ',
  usage: '{input} トークン入力 · {output} トークン出力',
  disclaimer: 'AI の回答は誤っている可能性があります。重要な数値はレポートで確認してください。金融・税務・法律上のアドバイスではありません。',
  image: {
    attach: 'レシート写真を添付',
    remove: '写真を削除',
    alt: 'レシート写真',
    defaultPrompt: 'このレシートを読み取り、記録方法を提案してください。',
    tooMany: '1回のメッセージに添付できる写真は最大{count}枚です。',
    invalid: '対応していない画像ファイルです（JPG、PNG、WebP）。'
  },
  action: {
    awaiting: '承認待ち',
    done: '保存済み',
    cancelled: 'キャンセル済み',
    expired: '期限切れ',
    kind: { petty_cash_add: '提案：小口現金の記録', petty_cash_void: '提案：小口現金の取り消し' },
    direction: '種類',
    dir: { out: '現金支出', in: '現金入金' },
    amount: '金額',
    category: 'カテゴリ',
    note: 'メモ',
    created: '記録日時',
    voidWarn: '取り消しはシフト締め時に数えるレジ現金に影響し、元に戻せません。',
    pinHint: 'PINはここで入力され、サーバーに直接送信されます。AIには送られません。',
    approve: '承認',
    cancel: 'キャンセル',
    working: '保存中…'
  },
  live: 'ONLINE',
  liveFree: '無料アカウント',
  hero: { eyebrow: 'ARUS Intelligence', tryThis: 'この質問例を入力欄に入れる' },
  thinkingSteps: { s1: '質問を理解中…', s2: '事業データを読み取り中…', s3: '計算・比較中…', s4: '結論をまとめ中…' },
  history: {
    title: '履歴',
    new: '新しい会話',
    empty: '会話履歴はまだありません。',
    more: 'さらに読み込む',
    rename: '名前を変更',
    delete: '削除',
    deleteConfirm: 'この会話を削除しますか？元に戻せません。',
    deleteAll: '自分の履歴をすべて削除',
    deleteAllConfirm: 'あなたの会話履歴をすべて削除しますか？元に戻せません。',
    deleted: '{count}件の会話を削除しました。',
    retention: '暗号化されており、あなただけが読めます。90日後に自動削除されます。',
    loading: '会話を開いています…',
    saving: '履歴に保存',
    temp: '一時的な会話（保存されません）',
    tempHint: 'クリックするとこの会話は一時的になり、履歴に保存されません。',
    tempLocked: '変更するには新しい会話を始めてください。',
    off: '履歴は事業オーナーによってオフにされています。会話は保存されません。',
    hadImages: 'レシート写真を添付（保存されません）',
    noPersonal: 'メッセージや写真に個人情報（氏名・番号）を書かないでください。',
    switchLabel: '会話履歴を保存する',
    switchHint: '暗号化され、会話の所有者だけが読めます。{days}日後に自動削除されます。レシート写真は保存されません。ユーザーは一時的な会話も使えます。',
    saved: '履歴の設定を保存しました。'
  },
  custom: {
    name: 'カスタムプロバイダー',
    add: '＋ カスタムプロバイダーを追加（OpenAI互換）',
    intro: 'OpenAI形式のAPI（/chat/completions）に対応していれば、OpenKeyやOpenRouterなどどのAIプロバイダーでも使えます。アドレス（ベースURL）、モデル名、APIキーを入力してください。キーは暗号化してサーバーに保存され、再表示はできません。HalloArus がデータを読むには、モデルがツール呼び出しに対応している必要があります。',
    labelField: 'プロバイダー名',
    labelPlaceholder: '例：OpenKey',
    baseUrl: 'ベースURL',
    baseUrlHint: 'httpsで公開アドレスのみ（localhostや内部ネットワークは不可）。例：https://free.openkey.id/v1',
    modelHint: 'プロバイダーが示すモデル名をそのまま入力してください。レシート写真は Claude プロバイダーでのみ読み取れます。',
    needFields: 'プロバイダー名、ベースURL、モデルを入力してください。',
    needKeyForUrl: 'ベースURLが変更されました。APIキーを再入力してください（古いキーは新しいアドレスには送信されません）。'
  },
  tools: {
    list_outlets: '店舗一覧',
    sales_summary: '売上サマリー',
    top_items: '売れ筋・不振商品',
    sales_by_category: 'カテゴリ別売上',
    returns_summary: '返品サマリー',
    member_overview: '顧客サマリー（匿名化）',
    stock_overview: '在庫状況',
    stock_items: '注意が必要な商品',
    slow_moving_items: '眠っている在庫',
    stock_opname_summary: '棚卸結果',
    receivables_summary: '売掛金と滞留期間',
    payables_summary: '仕入先買掛金',
    purchases_summary: '仕入サマリー',
    cash_shifts_summary: 'レジシフト管理',
    petty_cash_summary: '小口現金',
    list_petty_cash: '小口現金の一覧',
    propose_petty_cash_entry: '小口現金の記録を提案',
    propose_void_petty_cash_entry: '小口現金の取り消しを提案',
    accounting_balances: '勘定残高（元帳）'
  }
};

export default ai;
