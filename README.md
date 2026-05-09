# 開発アプリケーション： (Eコマース名:OmniCommerceCore)<br>
# Stateful〜AWS+AuthO認証、DDD、マルチテナント、AI管理、 決済マルチゲートウェイ、（自動フルフィルメントなども計画）システム<br><br>

# COACHTECHで学んだこと、よりプラス独学での思考しての実装実績<br>
DDD、マルチテナント、購入されてから出品完了までの手動反映機能、<br>
Stateful認証〜AWS+AuthO認証、ゼロトラスト認証まで可能な継続システム設計<br>
決済マルチゲートウェイ(strip/adyen),クレジットカードを保管記録してのワンクリック決済、<br>
AIシステム(出品自動補完管理、現時点でブランド、状態、色のテキスト自動補完管理)<br>
多言語実装(PHP:Python:Go:Ruby:Java:JavaScript:Gradle Kotlin:Sql)<br><br>


リポジトリ名: - research-stateful-auth-archaeology<br>
ブランチ名:　 - main<br>

それぞれのブランチのREADMEを参照してセットアップ<br>

# 環境構築　＋　実装機能確認準備
<br>
Dockerビルド
<br>
<br>
　1\. git cloneで作成（ターミナルコマンド）新規作成したディレクトリでcdで移動してから実行<br>
 git clone https://github.com/takayuki-develop2026/research-stateful-auth-archaeology.git  の実行<br>

　2\. （ターミナルコマンド）cd research-stateful-auth-archaeology  の実行<br><br>

　3\. ダミーデーターの商品画像ファイルをstrageディレクトリーの中にitem_imagesディレクトリーを作成して商品画像ファイルをコピーする。<br>
　　（ターミナルコマンド）cd backend (実行後) mkdir storage/app/public/item_images　の実行<br>
　　　　　　　　　　　cp -r public/pictures/* storage/app/public/item_images　の実行<br>
　4\. ダミーデーターのユーザー初期画像ファイルをstrageディレクトリーの中にimagesディレクトリーを作成して初期画像ファイルをコピーする<br>
　（ターミナルコマンド）mkdir storage/app/public/images　の実行<br>
　　　　　　　　　　　cp -r public/pictures_user/* storage/app/public/images　の実行<br><br>

　5\. env.exampleファイルから.envを作成(backend+frontend+admin_rails)し、.envファイルの環境変数を変更(backend)<br>
　　a:(backendディレクトリで実行) cp .env.example .env　の実行後.envの環境変数の変更<br>
　DB_PASSWORD="",と<br>
　AUTH0_MANAGEMENT_CLIENT_SECRET="",<br>
　(新規登録時403エラーになるのはメール認証完了してないとログインできない仕様だからです。)<br>
　　b:(frontendディレクトリで実行) cp .env.example .env　の実行<br>
　　c:(admin_railsディレクトリで実行) cp .env.example .env 2>/dev/null || true && test -f .env || touch .env<br>
　の実行<br>

　 シークレットキーなどは個人情報保護のためgitで追跡していません。必要でしたら伝えます。<br>
(backend)<br>
FIREBASE_CREDENTIALS=(現段階では使っていない。jsonファイルなし位置表示のみ),<br>
JWT_SECRET=,DB_PASSWORD=,<br>
AUTH0_MANAGEMENT_CLIENT_SECRET=,<br>
(STRIPE)STRIPE_KEY=,STRIPE_SECRET=,STRIPE_WEBHOOK_SECRET=,<br>
(ADYEN)ADYEN_API_KEY=,ADYEN_HMAC_KEY=,　　は空です。(////)を削除して各準備お願いします。<br>
  必要でしたら　backend .env　追記用　と　./backend/config/firebase-service-account.json　ファイルに必要なコード伝えます。<br><br>

　6\. Docker Desktopを立ち上げ(今回一度に立ち上げるとエラーになるので部分的から)<br>
（カレントディレクトリー）docker compose up -d ak_postgres ak_redis mysql php frontend_dev oracle nginx　admin_rails の実行
<br><br>

laravel環境構築
<br>
<br>
　1\. (カレントディレクトリー)docker-compose exec php bash　の実行
<br>
　2\. （PHPコンテナー）composer install　の実行
<br>
　3\. アプリケーションキーの作成<br>
　　（PHPコンテナー）sed -i '/^APP_KEY=/d' .env
php -r "echo 'APP_KEY=base64:'.base64_encode(random_bytes(32)).PHP_EOL;" >> .env
<br>
　4\. マイグレーションの実行・シーディング実行<br>
　　（PHPコンテナー）php artisan migrate:fresh --seed
<br>
　5\. シンボリックリンクの作成<br>
　　（カレントディレクトリー）docker compose exec php sh -lc 'cd /var/www/backend && php artisan storage:link'
<br>
　6\. フロントエンドのセットアップ。<br>
　　 (frontendディレクトリー)npm i　の実行
<br><br>

-  シーダーファイルでユーザーデーターと出品商品データーを作成しました。<br>
   ユーザー情報です。メールの'　'は削除してください。<br><br>

   １：名前:'テスト用のユーザ１'、アドレス:　'valid.email@example.com'　パスワード:　'Testtest1'　出品数：'２品'<br>
   ロール：Shop Owner（各Shop Owner(manager、staff)は<br>
   ログイン後それぞれのショップのダッシュボードに移動します。）<br>
   (各 テストショップのトップページから管理画面ボタンで各ショップのダッシュボードに入れます。)<br>
   ２：名前:'テスト用のユーザ2'、アドレス:　'taro.y@coachtech.com'　パスワード:　'Testtest2'　出品数：'２品'　ロール：Shop Owner<br>
   ３：名前:'テスト用のユーザ3'、アドレス:　'reina.n@coachtech.com'　パスワード:　'Testtest3'　出品数：'３品'　ロール：Shop Owner<br>
   ４：名前:'テスト用のユーザ4'、アドレス:　'tomomi.a@coachtech.com'　パスワード:　'Testtest4'　出品数：'３品'　ロール：Shop Owner<br>
   ５：名前:'テスト用のユーザ5'(管理者)、アドレス:　'pro.t@coachtech.com'　パスワード:　'Testtest5'　出品数：'0'　ロール：Domain Lead Admin
   　です。(こちらのログインでショップ全体のAtlaskernelの画面が見れます。)<br>
   6：名前:'川田　隆之'(管理者)、アドレス:　't.principle.k2024@gmail.com'　パスワード:　'git hub　ログイン'　出品数：'0'　ロール：Domain Lead Admin
   　です。(こちらのログインでショップ全体のAtlaskernelの画面が見れます。)<br>
   １から６はショップオーナー、管理者ですが、新規登録でカスタマーユーザー作成できます。(新規登録の流れをアニメーション動画で作成しています。)<br><br>


<h2>PISAG（ピサグ）システムの機能確認&Dockerのfull起動順序</h2>

この確認手順は、<strong>PISAG の migration・DB function・単発 worker・常駐 service が正常に動くか</strong>を、段階的に確認するためのものです。<br>
このプロジェクトは、いきなり full 起動せず、<strong>最小構成で初期化してから段階的に起動</strong>してください。<br>
初期化前に全サービスを起動すると、空の DB に常駐 service / worker が接続し、不整合や起動失敗の原因になります。<br>
<br>

<h2>事前注意</h2>

・<code>ak_go_worker</code> / <code>ak_go_worker_2</code> は <strong>legacy worker</strong> です。通常の <code>docker compose up -d</code> では起動しません。<br>
・PISAG の確認は、まず常駐 worker ではなく <code>./scripts/run_pisag_worker_once.sh</code> を使ってください。<br>
・<code>admin_rails/.env</code> の Firebase API Key が不正だと、フロントエンドで <code>auth/invalid-api-key</code> が発生します。<br>
・このプロジェクトは、<strong>最小構成で初期化 → DB ユーザー・テーブル作成 → PISAG 単発確認 → full 起動</strong> の順で進めてください。<br>
<br>

<h2>1. 最小構成を起動</h2>

まずは DB・Laravel・フロントの基盤だけを立ち上げます。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>docker compose up -d ak_postgres ak_redis mysql php frontend_dev oracle nginx</code><br>
<br>

<h2>2. 必須 DB ユーザー（Role）の作成</h2>

<code>decisioncoresvc</code> などの service が <code>ak_postgres</code> に接続できるよう、必要な DB ユーザーを事前に作成します。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>docker compose exec -T ak_postgres psql -U ak -d ak -c "DO \$\$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncoresvc') THEN CREATE ROLE decisioncoresvc LOGIN PASSWORD 'decisioncoresvc'; END IF; IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncore_worker') THEN CREATE ROLE decisioncore_worker LOGIN PASSWORD 'decisioncore_worker'; END IF; END \$\$;"</code><br>
<br>
これが不足していると、<code>Role "decisioncoresvc" does not exist</code> で service が起動できません。<br>
<br>

<h2>3. ak_postgres 側 migration 適用</h2>

<code>pisag_go/migrations/*.sql</code> を順に流して、必要なテーブル・関数・権限を作成します。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>for f in ./pisag_go/migrations/*.sql; do</code><br>
<code>&nbsp;&nbsp;echo "Applying $f..."</code><br>
<code>&nbsp;&nbsp;docker compose exec -T ak_postgres psql -v ON_ERROR_STOP=1 -U ak -d ak &lt; "$f" || break</code><br>
<code>done</code><br>
<br>

<h2>4. migration / function の確認</h2>

まず、PISAG が必要とするテーブルと function が作成されているかを確認します。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>docker compose exec -T ak_postgres psql -U ak -d ak -c "\dt public.*"</code><br>
<code>docker compose exec -T ak_postgres psql -U ak -d ak -c "\df public.run_inputs_claim_next*"</code><br>
<br>

<strong>確認ポイント</strong><br>
・<code>public</code> schema のテーブル一覧が表示される<br>
・<code>run_inputs_claim_next</code> function が表示される<br>
<br>
※ <code>\dt public.*</code> と <code>\df ...</code> は、<strong>1行ずつ別々に実行</strong>してください。<br>
<br>

<h2>5. PISAG 単発動作確認</h2>

常駐 worker ではなく、単発 script で 1 回分の処理を確認します。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>./scripts/run_pisag_worker_once.sh</code><br>
<code>./scripts/check_pisag_db_state.sh</code><br>
<br>

<strong>成功の目安</strong><br>
・<code>run_inputs.claim_status</code> が <code>done</code><br>
・worker ログに <code>done: input_id=... status=200</code> が出る<br>
・<code>run_evidence_assets</code> にデータが入る<br>
・<code>run_evidence_manifests</code> に manifest 情報が入る<br>
<br>

<strong>補足</strong><br>
・<code>idle: no pending input</code> だけの場合は、処理対象が無かっただけの可能性があります。<br>
・この場合は異常確定ではなく、<code>check_pisag_db_state.sh</code> の結果も合わせて確認してください。<br>
<br>

<h2>6. 通常の full 起動</h2>

ここまで通ったら、通常の full 起動を行います。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>docker compose up -d</code><br>
<br>

<strong>補足</strong><br>
この通常起動では、次の legacy worker は起動しません。<br>
・<code>ak_go_worker</code><br>
・<code>ak_go_worker_2</code><br>
<br>
これは意図的な挙動です。<br>
これらは現行 DB 仕様と不整合になる可能性があるため、通常運用から外しています。<br>
<br>

<h2>7. 起動確認</h2>

<strong>（ターミナル実行）</strong><br>
<code>docker compose ps</code><br>
<code>docker compose logs --tail=50 decisioncoresvc decisioncore_worker runschedsvc v22_ocr_daemon</code><br>
<br>

<strong>チェックポイント</strong><br>
・<code>decisioncoresvc</code> が <code>listening on :9023</code> になっている<br>
・<code>v22_ocr_daemon</code> が <code>start</code> している<br>
・<code>decisioncore_worker</code> が <code>Restarting</code> ではなく <code>Up</code> を維持している<br>
・<code>runschedsvc</code> が動作している<br>
<br>

<strong>runschedsvc について</strong><br>
<code>force_budget_deny=true</code> により dispatch が空振りすることがありますが、これは即異常とは限りません。<br>
<code>summary created=0 skipped=0 errors=0</code> のようなログで安定していれば、致命停止ではありません。<br>
<br>

<h2>8. legacy worker が必要な場合のみ</h2>

通常は不要です。必要な場合だけ profile 指定で起動してください。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>docker compose --profile legacy-workers up -d ak_go_worker ak_go_worker_2</code><br>
<br>

<strong>注意</strong><br>
legacy worker は、現時点では <code>run_status: "queued"</code> 関連エラーを出す可能性があるため、通常運用では非推奨です。<br>
<br>

<h2>9. トラブルシューティング</h2>

<strong>Role "decisioncoresvc" does not exist</strong><br>
手順 2 の必須 DB ユーザー（Role）の作成が未実施の可能性があります。<br>
再度 SQL を実行してください。<br>
<br>

<strong>Firebase: Error (auth/invalid-api-key)</strong><br>
<code>admin_rails/.env</code> または関連 env の Firebase API Key が不正です。<br>
既存の正常環境の値と揃えてください。<br>
<br>

<strong>ak_go_worker / ak_go_worker_2 がエラーを吐く</strong><br>
通常起動からは外しているため、<code>docker compose up -d</code> では起動しないのが正しい状態です。<br>
profile 指定で明示的に起動した場合のみ対象になります。<br>
<br>

<strong>DB 接続系の起動失敗</strong><br>
まず以下を確認してください。<br>
<br>
<strong>（ターミナル実行）</strong><br>
<code>docker compose ps</code><br>
<code>docker compose logs --tail=100 ak_postgres decisioncoresvc decisioncore_worker</code><br>
<br>

<h2>10. 手元確認用の一括コマンド（任意）</h2>

必要に応じて、以下で最小構成の確認から full 起動までまとめて確認できます。<br>
ただし、通常は <strong>1〜9 の段階実行を推奨</strong>します。<br>
<br>

<pre><code>cd /path/to/research-stateful-auth-archaeology

docker compose up -d ak_postgres ak_redis mysql php frontend_dev oracle nginx

docker compose exec -T ak_postgres psql -U ak -d ak -c "DO \$\$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncoresvc') THEN
    CREATE ROLE decisioncoresvc LOGIN PASSWORD 'decisioncoresvc';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncore_worker') THEN
    CREATE ROLE decisioncore_worker LOGIN PASSWORD 'decisioncore_worker';
  END IF;
END \$\$;"

for f in ./pisag_go/migrations/*.sql; do
  echo "Applying $f..."
  docker compose exec -T ak_postgres psql -v ON_ERROR_STOP=1 -U ak -d ak &lt; "$f" || break
done

./scripts/run_pisag_worker_once.sh
./scripts/check_pisag_db_state.sh

docker compose up -d
docker compose ps</code></pre>

<h2>この確認手順で何を保証するか</h2>

この確認手順では、<strong>PISAG が確実に機能していると言える条件</strong>を次の 3 段階で確認します。<br>
・<strong>DB 構造確認</strong>：migration と function が入っている<br>
・<strong>単発 worker 確認</strong>：PISAG が 1 回分の処理を完了できる<br>
・<strong>常駐系確認</strong>：full 起動後に関連 service が正常稼働している<br><br>


- AI解析システム（出品解析システム：出品する前に実行） <br>
（ターミナルコマンド）docker compose exec php php artisan queue:work(ターミナルで実行のまま)<br><br>

- Stripe決済実行前、事前にパソコンにインストール必要（brew install stripe/stripe-cli/stripe）<br>
（ターミナルコマンド）stripe listen --forward-to http://localhost/api/webhooks/stripe (ターミナルで実行のまま)<br>
カード番号：4242 4242 4242 4242<br>
有効期限（未来）・シークレットナンバー・名前、は決まりなし。<br>
コンビニ払いは現在Stripeのみで決済後3分ほどでダッシュボードに反映<br><br>

- Adyen決済実行前、事前にパソコンにインストール必要（brew install ngrok/ngrok/ngrok）<br>
（ターミナルコマンド）ngrok http 80  (ターミナルで実行のまま)<br>
カード番号：4111 1111 1111 1111 /シークレットナンバー：737<br>
有効期限（未来）・名前、は決まりなし。<br><br>


- 光学文字認識機能<br>
(ターミナル実行)cd /Users/kawadatakayuki/research-stateful-auth-archaeology/pisag_go && go run ./cmd/v22_runtime_api　の実行　＋　Docker全て起動した状態で機能します。<br>
　ログインページの管理者/開発者コンソールへ（admin/dashboard）（開発用）に入り、<br>
 OCR(簡易(開発途中)光学文字認識)に移動して、ファイルを選択して、Engine Selectionを選択して、<br>
 Run AI Runtimeを押してください。<br>
 少し時間がかかる時もありそのページでリロードすると結果が表示されます。
<br><br>

# アプリの仕様計画<br>
・Adminでショップ運営の権限を与えることができて(ShopOwner付与)<br>
ShopOwnerからManageとStaffの権限を与えることができる。(個人も申請すれば出店できる)<br>
・出品商品のマークは、<br>
カスタマー出品、ショップユーザーが個人出品の場合は💫、ショップの管理商品は⭐️となる<br>


# 次のステップ提案<br>





# 伝えること<br>
-  stripe決済の都合上最低決済金額が50円なので少し余裕を持たせて出品商品の最低金額を100円以上(変更)にして設定しました。（バリデーション、テスト含む）<br>
-  カード支払いで商品購入処理完了後に登録したstripeのdashboardを参照すれば処理が成功しているのが分かります。必要があれば伝えます。<br><br>
-  COACHTECHのロゴをクリックするとトップページに、ログインユーザーが商品詳細画面で自分が出品した商品の購入手続きをクリックするとプロフィールページに、ゲストユーザーが購入手続きへ・ヘッダーのマイページ・出品・コメントを送信するをクリックするとログインページに移動するようになっています。<br><br>
-  いいね機能はゲストユーザー、ログインユーザーの自分の出品した商品にはできないようになっています。<br><br>
-  コメント機能はログインユーザーが商品を見てコメントする時と、出品者が出品後に追加でコメントした日時がわかるようにしました。<br><br>
-  PHPUnitのテストファイルはスプレットシートのテストケース一覧のID番号に沿ってtests/Featureディレクトリーに保存してあります。上記に記したテスト用のデーターベースを作成した後phpコンテナーで php artisan test を実行してテストをしてください。 <br><br>
-  Route,Controllerは基本設計書に沿ってファイルの中に基本並び替えしています。<br><br>
-  ダミーのユーザーデーターと出品商品データーのシーダーファイルで作りましたので、PHPコンテナーで上記の通り　php artisan db:seed　を実行してください。<br>
-  プロフィールのユーザー画像を登録していない場合は初期画面として、default-profile２.jpgファイルの画像を使っています。それからユーザー、商品画像を登録した際は同じファイル名にならないよう頭文字以外はランダムで生成するようにしました。<br><br>
-  スプレットシートの機能要件一覧（US006 FN022.4）の商品を購入した後の還移先は商品一覧画面のところを一つ挟んで購入完了画面を追加しました。その後ページのトップページに戻るを押すと商品一覧画面に移動します。商品を出品した後は出品完了画面に移動してトップページに戻るを押すと商品一覧画面に移動します。<br><br>
-  出品商品の商品名,ブランド名の文字数は２０文字以内、金額は２０億円以内（バリデーション、テスト含む）に設定しました。<br><br>
<br>
<br>

# スプレットシートの基本設計書にある項目で追加した内容（模擬案件の時だけ掲載）<br><br>
- 画面関係のRoute,Controller<br>
　　出品完了や画面や処理：パス・/thanks_sell　アクション名・thanks_sell_create<br>
　　購入完了画面や処理：パス・/thanks_buy　アクション名・thanks_buy_create<br>
　　email認証通知画面や処理:パス・/email/verify　アクション名・notice/verify/resend<br>
　　stripe決済の処理：パス・/stripe_success　アクション名・stripeSuccess<br>
　　追加コントローラー名：EmailVerificationController（認証メール処理のコントローラー）<br><br>
- Viewファイル<br>
　　出品完了画面：thanks_sell.blade.php<br>
　　購入完了画面：thanks_buy.blade.php<br>
　　email認証通知画面：verify-email.blade.php<br>
　　stripeカード支払い決済画面：　stripe機能が提供<br><br>
- バリデーション関係<br>
　　追加　ファイル名：ProfileImageRequest.php　内容・ユーザー画像アップロード　ルール・拡張子が.jpegもしくは.png<br>
　　　（ProfileRequest.phpのプロフィール画像だけこちらに作成しました。）<br>
　　変更（RegisterRequest.phpは作成せずにCreateNewUser.phpとlang/ja/validation.php）を修正してfortifyの機能でバリデーションしました。<br>
　　変更（LoginRequest.phpは作成せずにlang/ja/validation.php）を修正してfortifyの機能でバリデーションしました。<br>
　　変更 ExhibitionRequest.phpの商品画像の拡張子のバリデーションはコントローラーで処理しています。（アップロード必須は設定してあります。）<br>

<br>
<br>

# 今後開発品質の高い効率の良いWEB開発をしていく上でのまとめ（模擬案件の時だけ掲載）<br>
- 　estra様の教育形態、ビジネスモデルがとても良く模擬案件１つ目のフリマアプリ制作でも幅広く学ばせていただいています。<br>
　商品出品機能から登録者情報（購入者情報）の登録・更新や付随する機能などを学び、<br>
「ユーザビリティ」、「アクセシビリティ」、「UI」、「UX」などを深く学びできるだけ新しい技術を取り入れたWEB開発を心がけて、納品してからの運用、保守もしやすい開発ができるように学んでいきます<br>
　フリマアプリ制作の応用にECサイト制作等あるのですが、ECサイトは企業や個人の個性が重要で<br>
WEBサイトにも良く反映されて景気の波にも負けないような技術を身につけられるように、案件の要件シート通りに（最初の要件シートの作成もできるように）制作して良い開発ができるように良い経験を積ませていただくことができればと思っています。<br>
　よろしくお願い致します。<br><br>


# ER図<br>


# 使用技術<br>
  - PHP: 8.4.17
  - Laravel: 11.47.0
  - Python: 3.14
  - Go: 1.25.1
  - ginkgo: 2.12.0
  - Ruby: 4.0.1
  - Rails: 8.1.2
  - Java: toolchain 25
  - Spring Boot: 4.0.1
  - Gradle: 9.3.0
  - Gradle Kotlin: 2.2.21
  - MySql: 8.3
  - Nginx: 1.21.1
  - Firebase: 10.12.2
  - React: 19.0.0
  - Next.js: 16.0.0
  - Node: 22-bullseye
<br>

# URL<br>
  - Eコマースアプリトップページ： http://localhost/
  - ユーザー登録： http://localhost/register
  - phpMyAdmin:http://localhost:8080/index.php
  - meilhog： http://localhost:8025/
  - cloudbeaver:http://localhost:8978/

