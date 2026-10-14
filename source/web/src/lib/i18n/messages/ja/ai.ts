import type { Messages } from '../../types.ts';

const ai: Messages['ai'] = {
  docTitle: 'HalloArus | ACIRABA',
  title: 'HalloArus',
  subtitle: 'ビジネスデータを読み取り、結論を導き出し、アドバイスを提供するAIアシスタント。',
  readOnly: '読み取り専用',
  readOnlyHint: 'HalloArus はデータの追加・変更・削除を行うことはできません。',
  privacy:
    '集計された数値は、選択したAIプロバイダー（Claude、ChatGPT、Gemini、Qwen）へ、事業所有のAPIキー（または無料のARUSアカウント）を使用して送信されます。顧客データ（氏名、電話番号、住所）は送信されません。顧客は「Pelanggan 1、2、…」としてのみ表示されます。',
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
    accounting_balances: '勘定残高（元帳）'
  }
};

export default ai;
