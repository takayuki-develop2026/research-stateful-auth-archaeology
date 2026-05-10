# 開発アプリケーション： (Eコマース名:OmniCommerceCore)<br>
# Stateful〜AWS+AuthO認証、DDD、マルチテナント、AI管理、 決済マルチゲートウェイ、PISAG（ピサグ）システム、（自動フルフィルメントなども計画）システム<br><br><br>

# COACHTECHで学んだこと、<br>プラス独学での思考しての実装実績<br>
DDD、マルチテナント、購入されてから出品完了までの手動反映機能、<br>
Stateful認証〜AWS+AuthO認証、ゼロトラスト認証まで可能な継続システム設計<br>
認証なども含めUX改善 / Auth0連携 / Next.js状態管理 / フロントエンドアニメーション / オンボーディング導線設計<br>
決済マルチゲートウェイ(strip/adyen),クレジットカードを保管記録してのワンクリック決済、<br>
AIシステム(出品自動補完管理、現時点でブランド、状態、色のテキスト自動補完管理)<br>
PISAG（ピサグ）システム<br>
多言語実装(PHP:Python:Go:Ruby:Java:JavaScript:Gradle Kotlin:Sql)<br><br>


リポジトリ名: - research-stateful-auth-archaeology<br>
ブランチ名:　 - main<br>

それぞれのブランチのREADMEを参照してセットアップ<br>

# 環境構築　＋　実装機能確認準備
<br>
<h2>git clone / Docker ビルド / 開発・閲覧 環境構築</h2>

<p>
この手順は、GitHub からプロジェクトを clone し、Docker 起動、Laravel 初期化、ダミーデータ用画像配置、フロントエンド準備までを行うためのものです。
</p>

<p>
コマンドは、指定されたディレクトリで順番に実行してください。
</p>

<hr>

<h2>1. git clone</h2>

<p>
新規作成したディレクトリに <code>cd</code> で移動してから、以下を実行します。
</p>

<pre><code>git clone https://github.com/takayuki-develop2026/research-stateful-auth-archaeology.git</code></pre>

<p>
clone 後、プロジェクトディレクトリへ移動します。
</p>

<pre><code>cd research-stateful-auth-archaeology</code></pre>

<hr>

<h2>2. ダミーデータの商品画像ファイルを配置</h2>

<p>
ダミーデータの商品画像ファイルを、Laravel の <code>storage</code> ディレクトリ内に配置します。
</p>

<p>
まず <code>backend</code> ディレクトリへ移動します。
</p>

<pre><code>cd backend</code></pre>

<p>
商品画像用ディレクトリを作成します。
</p>

<pre><code>mkdir -p storage/app/public/item_images</code></pre>

<p>
商品画像ファイルをコピーします。
</p>

<pre><code>cp -r public/pictures/* storage/app/public/item_images</code></pre>

<hr>

<h2>3. ダミーデータのユーザー初期画像ファイルを配置</h2>

<p>
そのまま <code>backend</code> ディレクトリで実行します。
</p>

<p>
ユーザー初期画像用ディレクトリを作成します。
</p>

<pre><code>mkdir -p storage/app/public/images</code></pre>

<p>
ユーザー初期画像ファイルをコピーします。
</p>

<pre><code>cp -r public/pictures_user/* storage/app/public/images</code></pre>

<hr>

<h2>4. .env ファイルを作成</h2>

<p>
<code>.env.example</code> から <code>.env</code> を作成します。
</p>

<h3>4-1. backend の .env 作成</h3>

<p>
<code>backend</code> ディレクトリで実行します。
</p>

<pre><code>cp .env.example .env</code></pre>

<p>
その後、<code>.env</code> の環境変数を必要に応じて変更してください。
</p>

<p>
特に以下を確認してください。
</p>

<pre><code>DB_PASSWORD=""
AUTH0_MANAGEMENT_CLIENT_SECRET=""</code></pre>

<p>
新規登録時に 403 エラーになる場合がありますが、これはメール認証が完了していないとログインできない仕様です。
</p>

<h3>4-2. frontend の .env 作成</h3>

<p>
<code>frontend</code> ディレクトリで実行します。
</p>

<pre><code>cd ../frontend
cp .env.example .env</code></pre>

<h3>4-3. admin_rails の .env 作成</h3>

<p>
<code>admin_rails</code> ディレクトリで実行します。
</p>

<pre><code>cd ../admin_rails
cp .env.example .env</code></pre>

<hr>

<h2>5. シークレットキー・外部サービスキーについて</h2>

<p>
シークレットキーなどは個人情報保護のため、Git では追跡していません。
必要でしたら別途共有します。
</p>

<h3>backend 側で空になっている可能性がある項目</h3>

<pre><code>FIREBASE_CREDENTIALS=
JWT_SECRET=
DB_PASSWORD=
AUTH0_MANAGEMENT_CLIENT_SECRET=

STRIPE_KEY=
STRIPE_SECRET=
STRIPE_WEBHOOK_SECRET=

ADYEN_API_KEY=
ADYEN_HMAC_KEY=</code></pre>

<p>
<code>FIREBASE_CREDENTIALS</code> は現段階では使用していません。
JSON ファイルなしの位置表示のみです。
</p>

<p>
必要でしたら、以下の内容を別途共有します。
</p>

<ul>
  <li><code>backend .env</code> 追記用の値</li>
  <li><code>./backend/config/firebase-service-account.json</code> に必要なコード</li>
</ul>

<hr>

<h2>6. Docker Desktop を起動</h2>

<p>
Docker Desktop を立ち上げてください。
</p>

<p>
今回は一度に全サービスを立ち上げるとエラーになる可能性があるため、まずは部分的に起動します。
</p>

<p>
プロジェクトルートで実行します。
</p>

<pre><code>cd /Users/kawadatakayuki/research-stateful-auth-archaeology

docker compose up -d ak_postgres ak_redis mysql php frontend_dev oracle nginx admin_rails</code></pre>

<hr>

<h2>7. Laravel 環境構築</h2>

<h3>7-1. PHP コンテナに入る</h3>

<p>
プロジェクトルートで実行します。
</p>

<pre><code>docker compose exec php bash</code></pre>

<h3>7-2. Composer install</h3>

<p>
PHP コンテナ内で実行します。
</p>

<pre><code>composer install</code></pre>

<h3>7-3. アプリケーションキーを作成</h3>

<p>
PHP コンテナ内で実行します。
</p>

<pre><code>sed -i '/^APP_KEY=/d' .env &amp;&amp; php -r "echo 'APP_KEY=base64:'.base64_encode(random_bytes(32)).PHP_EOL;" &gt;&gt; .env &amp;&amp; php artisan config:clear &amp;&amp; php artisan cache:clear</code></pre>

<p>
もし <code>php artisan cache:clear</code> で権限エラーが出る場合は、以下を実行してください。
</p>

<pre><code>mkdir -p storage/framework/cache/data storage/framework/sessions storage/framework/views storage/logs bootstrap/cache

chown -R www-data:www-data storage bootstrap/cache

chmod -R ug+rwX storage bootstrap/cache

php artisan cache:clear
php artisan config:clear
php artisan route:clear
php artisan view:clear</code></pre>

<h3>7-4. マイグレーション・シーディングを実行</h3>

<p>
PHP コンテナ内で実行します。
</p>

<pre><code>php artisan migrate:fresh --seed</code></pre>

<h3>7-5. シンボリックリンクを作成</h3>

<p>
プロジェクトルートで実行します。
</p>

<pre><code>docker compose exec php sh -lc 'cd /var/www/backend &amp;&amp; php artisan storage:link'</code></pre>

<hr>

<h2>8. フロントエンドのセットアップ</h2>

<p>
<code>frontend</code> ディレクトリで実行します。
</p>

<pre><code>cd frontend

npm i</code></pre>

<hr>

<h2>9. シーダーデータについて</h2>

<p>
シーダーファイルで、ユーザーデータと出品商品データを作成しています。
</p>

<p>
メールアドレスの前後に表示されている引用符 <code>' '</code> は、実際の入力時には削除してください。
</p>

<h3>テストユーザー一覧</h3>

<ul>
  <li>
    名前: <code>テスト用のユーザ１</code><br>
    アドレス: <code>valid.email@example.com</code><br>
    パスワード: <code>Testtest1</code><br>
    出品数: <code>2品</code><br>
    ロール: <code>Shop Owner</code>
  </li>
  <li>
    名前: <code>テスト用のユーザ2</code><br>
    アドレス: <code>taro.y@coachtech.com</code><br>
    パスワード: <code>Testtest2</code><br>
    出品数: <code>2品</code><br>
    ロール: <code>Shop Owner</code>
  </li>
  <li>
    名前: <code>テスト用のユーザ3</code><br>
    アドレス: <code>reina.n@coachtech.com</code><br>
    パスワード: <code>Testtest3</code><br>
    出品数: <code>3品</code><br>
    ロール: <code>Shop Owner</code>
  </li>
  <li>
    名前: <code>テスト用のユーザ4</code><br>
    アドレス: <code>tomomi.a@coachtech.com</code><br>
    パスワード: <code>Testtest4</code><br>
    出品数: <code>3品</code><br>
    ロール: <code>Shop Owner</code>
  </li>
  <li>
    名前: <code>テスト用のユーザ5</code>（管理者）<br>
    アドレス: <code>pro.t@coachtech.com</code><br>
    パスワード: <code>Testtest5</code><br>
    出品数: <code>0</code><br>
    ロール: <code>Domain Lead Admin</code><br>
    このログインでショップ全体の AtlasKernel 画面が見れます。
  </li>
  <li>
    名前: <code>川田 隆之</code>（管理者）<br>
    アドレス: <code>t.principle.k2024@gmail.com</code><br>
    パスワード: <code>git hub ログイン</code><br>
    出品数: <code>0</code><br>
    ロール: <code>Domain Lead Admin</code><br>
    このログインでショップ全体の AtlasKernel 画面が見れます。
  </li>
</ul>

<p>
各 Shop Owner は、ログイン後それぞれのショップのダッシュボードに移動します。
</p>

<p>
各テストショップのトップページから、管理画面ボタンで各ショップのダッシュボードに入れます。
</p>

<p>
1 から 6 はショップオーナー、または管理者ユーザーです。
新規登録でカスタマーユーザー作成できます。
</p>

<hr>

<h2>10. 新規登録・メール認証フローについて</h2>

<p>
新規登録の待ち時間の流れは、Next.js / React / CSS Modules で Auth0/OIDC 認証コールバック画面として実装しています。
</p>

<p>
メール認証フェーズを URL query・localStorage・sessionStorage で管理し、returnTo / nonce 制御、10秒 hold、手動続行、自動遷移、状態連動アニメーションを統合したオンボーディング UI を構築しています。
</p>

<hr>

<h2>11. 補足</h2>

<ul>
  <li>Shop Owner / manager / staff は、ログイン後それぞれのショップのダッシュボードに移動します。</li>
  <li>Domain Lead Admin は、ショップ全体の AtlasKernel 画面を確認できます。</li>
  <li>シークレットキーや外部サービスキーは Git で追跡していません。</li>
  <li>必要な場合は、backend の <code>.env</code> 追記用の値や Firebase service account 用 JSON を別途共有します。</li>
</ul>


<h2>PISAG（ピサグ）システムの機能確認 & Docker full 起動順序</h2>

<p>
この手順は、PISAG の DB migration / DB function / fetch worker / evidence / manifest / run lifecycle / run_events が正常に動作するかを確認するためのものです。
</p>

<p>
このプロジェクトは、いきなり full 起動せず、最小構成で DB を初期化してから段階的に起動してください。
</p>

<hr>

<h2>事前注意</h2>

<ul>
  <li><code>ak_go_worker</code> / <code>ak_go_worker_2</code> は legacy worker です。通常起動では使いません。</li>
  <li>現行の PISAG 確認には <code>pisag_fetch_worker</code> を使います。</li>
  <li><code>pisag_fetch_worker</code> は <code>./pisag_go</code> を <code>/app</code> に bind mount し、<code>go run ./cmd/ak_go_worker</code> で現在の Go ソースを直接起動します。</li>
  <li>Oracle fixture への HTTPS fetch には <code>ORACLE_CA_PATH=/app/certs/oracle.crt</code> が必要です。</li>
  <li><code>./docker/nginx/ssl:/app/certs:ro</code> を mount し、コンテナ内で <code>/app/certs/oracle.crt</code> が見える状態にしてください。</li>
  <li><code>admin_rails/.env</code> の Firebase API Key が不正だと、フロントエンドで <code>auth/invalid-api-key</code> が発生します。</li>
</ul>

<hr>

<h2>1. 最小構成を起動</h2>

<p>まず DB / Redis / Laravel / frontend / oracle / nginx を起動します。</p>

<pre><code>cd /Users/kawadatakayuki/research-stateful-auth-archaeology

docker compose up -d ak_postgres ak_redis mysql php frontend_dev oracle nginx</code></pre>

<hr>

<h2>2. 必須 DB Role を作成</h2>

<p>
<code>decisioncoresvc</code> などが Postgres に接続できるように、必要な Role を作成します。
</p>

<pre><code>docker compose exec -T ak_postgres psql -U ak -d ak &lt;&lt;'SQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncoresvc') THEN
    CREATE ROLE decisioncoresvc LOGIN PASSWORD 'decisioncoresvc';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'decisioncore_worker') THEN
    CREATE ROLE decisioncore_worker LOGIN PASSWORD 'decisioncore_worker';
  END IF;
END
$$;
SQL</code></pre>

<hr>

<h2>3. migration を適用</h2>

<pre><code>cd /Users/kawadatakayuki/research-stateful-auth-archaeology

for f in ./pisag_go/migrations/*.sql; do
  echo "Applying $f..."
  docker compose exec -T ak_postgres psql -v ON_ERROR_STOP=1 -U ak -d ak &lt; "$f" || break
done</code></pre>

<hr>

<h2>4. DB table / function を確認</h2>

<p>以下は 1 行ずつ別々に実行してください。</p>

<pre><code>docker compose exec -T ak_postgres psql -U ak -d ak -c "\dt public.*"</code></pre>

<pre><code>docker compose exec -T ak_postgres psql -U ak -d ak -c "\df public.run_inputs_claim_next*"</code></pre>

<pre><code>docker compose exec -T ak_postgres psql -U ak -d ak -c "\df public.runs_mark*"</code></pre>

<h3>確認ポイント</h3>

<pre><code>public schema のテーブルが表示される
run_inputs_claim_next が表示される
runs_mark_done / runs_mark_failed が表示される</code></pre>

<hr>

<h2>5. legacy worker が残っていないことを確認</h2>

<p>
<code>ak_go_worker</code> / <code>ak_go_worker_2</code> は legacy worker です。通常は起動しません。
</p>

<p>過去の orphan container が残っている場合は削除します。</p>

<pre><code>docker stop research-stateful-auth-archaeology-ak_go_worker-1 2&gt;/dev/null || true
docker rm research-stateful-auth-archaeology-ak_go_worker-1 2&gt;/dev/null || true

docker stop research-stateful-auth-archaeology-ak_go_worker_2-1 2&gt;/dev/null || true
docker rm research-stateful-auth-archaeology-ak_go_worker_2-1 2&gt;/dev/null || true</code></pre>

<hr>

<h2>6. pisag_fetch_worker の定義を確認</h2>

<pre><code>docker compose -f ./docker-compose.yml config | grep -n "pisag_fetch_worker" -A45</code></pre>

<p>最低限、以下が入っていることを確認します。</p>

<pre><code>image: golang:1.25-bookworm
command: go run ./cmd/ak_go_worker
AK_CLAIM_STYLE: cte_skip_locked
AK_EVIDENCE_DIR: /app/var/evidence
ORACLE_CA_PATH: /app/certs/oracle.crt
./pisag_go:/app
./docker/nginx/ssl:/app/certs:ro</code></pre>

<hr>

<h2>7. Oracle CA 証明書を確認</h2>

<pre><code>ls -lah ./docker/nginx/ssl</code></pre>

<h3>期待値</h3>

<pre><code>oracle.crt
oracle.key
localhost.pem
localhost-key.pem</code></pre>

<hr>

<h2>8. pisag_fetch_worker を起動</h2>

<pre><code>docker compose -f ./docker-compose.yml up -d --force-recreate pisag_fetch_worker</code></pre>

<h3>起動確認</h3>

<pre><code>docker compose -f ./docker-compose.yml ps pisag_fetch_worker</code></pre>

<pre><code>docker compose -f ./docker-compose.yml logs --tail=120 pisag_fetch_worker</code></pre>

<h3>期待ログ</h3>

<pre><code>oracle CA loaded: /app/certs/oracle.crt
boot: build=ak-go-worker-v4.5-manifest-links ...
worker started: id=pisag-fetch-worker-1...
idle: no pending input</code></pre>

<hr>

<h2>9. コンテナ内で CA mount を確認</h2>

<pre><code>docker compose -f ./docker-compose.yml exec pisag_fetch_worker sh -lc '
echo ORACLE_CA_PATH=$ORACLE_CA_PATH
ls -lah /app/certs
test -f /app/certs/oracle.crt &amp;&amp; echo "oracle.crt OK" || echo "oracle.crt missing"
'</code></pre>

<h3>期待値</h3>

<pre><code>ORACLE_CA_PATH=/app/certs/oracle.crt
oracle.crt OK</code></pre>

<hr>

<h2>10. PISAG 検証 run を再投入</h2>

<p>開発環境限定で、既存の検証 run を再投入します。</p>

<h3>検証用 run_id</h3>

<pre><code>c31cbf28-9375-47f4-b3b2-ea27ac1af643</code></pre>

<h3>再投入コマンド</h3>

<pre><code>docker compose -f ./docker-compose.yml exec -T ak_postgres psql -U ak -d ak -c "
delete from run_events
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';

delete from run_evidence_manifests
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';

delete from run_evidence_assets
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';

update run_inputs
set claim_status = 'pending',
    claimed_at = null,
    claimed_by = null,
    next_attempt_at = now(),
    last_error_code = null,
    last_error_message = null
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';

update runs
set status = 'running',
    finished_at = null,
    error_code = null,
    error_message = null,
    updated_at = now()
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';
"</code></pre>

<hr>

<h2>11. worker ログ確認</h2>

<pre><code>docker compose -f ./docker-compose.yml logs --tail=160 pisag_fetch_worker</code></pre>

<h3>期待ログ</h3>

<pre><code>done: input_id=1 run_id=c31cbf28-9375-47f4-b3b2-ea27ac1af643 trace_id=... status=200 body_bytes=147 body_sha=... manifest_id=... manifest_hash=...</code></pre>

<hr>

<h2>12. runs の完了確認</h2>

<pre><code>docker compose -f ./docker-compose.yml exec -T ak_postgres psql -U ak -d ak -c "
select
  run_id,
  status,
  finished_at,
  error_code,
  error_message,
  updated_at
from runs
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';
"</code></pre>

<h3>期待値</h3>

<pre><code>status = done
finished_at = not null
error_code = null
error_message = null</code></pre>

<hr>

<h2>13. evidence assets 確認</h2>

<pre><code>docker compose -f ./docker-compose.yml exec -T ak_postgres psql -U ak -d ak -c "
select
  id,
  kind,
  byte_size,
  sha256,
  final_url,
  stored_path,
  created_at
from run_evidence_assets
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643'
order by id;
"</code></pre>

<h3>期待値</h3>

<pre><code>fetch_body
fetch_meta
fetch_headers</code></pre>

<hr>

<h2>14. manifest 確認</h2>

<pre><code>docker compose -f ./docker-compose.yml exec -T ak_postgres psql -U ak -d ak -c "
select
  manifest_id,
  run_id,
  status,
  manifest_hash,
  created_at,
  updated_at
from run_evidence_manifests
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643';
"</code></pre>

<h3>期待値</h3>

<pre><code>status = complete
manifest_hash = 64文字の sha256 hash</code></pre>

<hr>

<h2>15. run_events 確認</h2>

<pre><code>docker compose -f ./docker-compose.yml exec -T ak_postgres psql -U ak -d ak -c "
select
  id,
  run_id,
  trace_id,
  event_name,
  step,
  status,
  message,
  data_json,
  created_at
from run_events
where run_id = 'c31cbf28-9375-47f4-b3b2-ea27ac1af643'
order by id;
"</code></pre>

<h3>期待値</h3>

<pre><code>event_name = run_finished
step       = pisag_fetch
status     = done
message    = PISAG fetch/evidence/manifest completed</code></pre>

<p><code>data_json</code> には以下が含まれます。</p>

<pre><code>input_id
target_url
final_url
http_status
body_bytes
body_sha256
manifest_id
manifest_hash</code></pre>

<hr>

<h2>16. PISAG 確認完了条件</h2>

<p>以下がすべて確認できれば、PISAG の主要経路は確認完了です。</p>

<pre><code>pisag_fetch_worker 起動 OK
Oracle CA 読み込み OK
HTTPS fetch 200 OK
run_inputs.claim_status = done
fetch_body / fetch_meta / fetch_headers evidence 作成 OK
run_evidence_manifests.status = complete
runs.status = done
runs.finished_at populated
run_events.event_name = run_finished
run_events.step = pisag_fetch
run_events.status = done</code></pre>

<h3>結論</h3>

<pre><code>PISAG fetch/evidence/manifest/run lifecycle/event logging path is verified.</code></pre>

<hr>

<h2>17. 通常の full 起動</h2>

<p>PISAG 確認後、full 起動します。</p>

<pre><code>docker compose up -d</code></pre>

<p>この通常起動では、次の legacy worker は起動しません。</p>

<pre><code>ak_go_worker
ak_go_worker_2</code></pre>

<p>
これは意図的な挙動です。これらは現行 DB 仕様と不整合になる可能性があるため、通常運用から外しています。
</p>

<hr>

<h2>18. full 起動後の確認</h2>

<pre><code>docker compose ps</code></pre>

<pre><code>docker compose logs --tail=50 decisioncoresvc decisioncore_worker runschedsvc v22_ocr_daemon</code></pre>

<h3>チェックポイント</h3>

<pre><code>decisioncoresvc が listening on :9023 になっている
v22_ocr_daemon が start している
decisioncore_worker が Restarting ではなく Up を維持している
runschedsvc が動作している</code></pre>

<p>
<code>runschedsvc</code> は <code>force_budget_deny=true</code> により dispatch が空振りすることがあります。
</p>

<p>
<code>summary created=0 skipped=0 errors=0</code> のようなログで安定していれば、即異常ではありません。
</p>

<hr>

<h2>19. legacy worker が必要な場合のみ</h2>

<p>通常は不要です。必要な場合だけ profile 指定で起動してください。</p>

<pre><code>docker compose --profile legacy-workers up -d ak_go_worker ak_go_worker_2</code></pre>

<h3>注意</h3>

<pre><code>legacy worker は run_status: "queued" 関連エラーを出す可能性があるため、通常運用では非推奨です。</code></pre>

<hr>

<h2>20. トラブルシューティング</h2>

<h3>Role "decisioncoresvc" does not exist</h3>

<p>手順 2 の DB Role 作成が未実施の可能性があります。</p>

<h3>go.mod requires go &gt;= 1.25</h3>

<p><code>pisag_fetch_worker</code> の image が古い可能性があります。</p>

<pre><code>image: golang:1.25-bookworm</code></pre>

<h3>tls: failed to verify certificate: x509: certificate signed by unknown authority</h3>

<p><code>ORACLE_CA_PATH</code> または cert volume が不足しています。</p>

<pre><code>docker compose -f ./docker-compose.yml exec pisag_fetch_worker sh -lc '
echo ORACLE_CA_PATH=$ORACLE_CA_PATH
ls -lah /app/certs
test -f /app/certs/oracle.crt &amp;&amp; echo "oracle.crt OK" || echo "oracle.crt missing"
'</code></pre>

<h3>idle: no pending input だけが出る</h3>

<p>
処理対象の <code>run_inputs</code> が無いだけの可能性があります。手順 10 の再投入コマンドで検証してください。
</p>

<h3>runs.status が running のまま</h3>

<pre><code>grep -n "RunRepo.MarkDone" ./pisag_go/internal/worker/worker.go</code></pre>

<h3>run_events が 0 rows のまま</h3>

<pre><code>grep -n "RunEventRepo" ./pisag_go/internal/worker/store.go</code></pre>

<pre><code>grep -n "RunEventRepo.Append\|run_finished" ./pisag_go/internal/worker/worker.go</code></pre>

<h3>Firebase: Error (auth/invalid-api-key)</h3>

<p>
<code>admin_rails/.env</code> または関連 env の Firebase API Key が不正です。既存の正常環境の値と揃えてください。
</p>

<h3>ak_go_worker / ak_go_worker_2 がエラーを吐く</h3>

<p>
通常起動からは外しているため、<code>docker compose up -d</code> では起動しないのが正しい状態です。
</p>

<p>profile 指定で明示的に起動した場合のみ対象です。</p>

<h3>DB 接続系の起動失敗</h3>

<pre><code>docker compose ps</code></pre>

<pre><code>docker compose logs --tail=100 ak_postgres decisioncoresvc decisioncore_worker</code></pre>

<hr>

<h2>21. この確認手順で保証すること</h2>

<p>この手順では、以下を確認します。</p>

<pre><code>DB 構造確認
DB function 確認
PISAG fetch worker 起動確認
Oracle HTTPS fetch 確認
evidence asset 作成確認
manifest complete 確認
runs.status done 確認
run_events.run_finished 確認
full 起動後の関連 service 稼働確認</code></pre>

<h3>最終結論</h3>

<pre><code>PISAG fetch/evidence/manifest/run lifecycle/event logging path is verified.</code></pre>


○ AI解析システム（出品解析システム：出品する前に実行） <br>
（ターミナルコマンド）docker compose exec php php artisan queue:work(ターミナルで実行のまま)<br><br>

○ Stripe決済実行前、事前にパソコンにインストール必要（brew install stripe/stripe-cli/stripe）<br>
（ターミナルコマンド）stripe listen --forward-to http://localhost/api/webhooks/stripe (ターミナルで実行のまま)<br>
カード番号：4242 4242 4242 4242<br>
有効期限（未来）・シークレットナンバー・名前、は決まりなし。<br>
コンビニ払いは現在Stripeのみで決済後3分ほどでダッシュボードに反映<br><br>

○ Adyen決済実行前、事前にパソコンにインストール必要（brew install ngrok/ngrok/ngrok）<br>
（ターミナルコマンド）ngrok http 80  (ターミナルで実行のまま)<br>
カード番号：4111 1111 1111 1111 /シークレットナンバー：737<br>
有効期限（未来）・名前、は決まりなし。<br><br>


○ 光学文字認識機能<br>
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
カスタマー出品、ショップユーザーが個人出品の場合は💫、ショップの管理商品は⭐️となる<br><br>

・PSP障害や決済手数料を考慮してAI探索管理やショップオーナーからの情報提供などをもとに、<br>
安全にPSP切り替えをできるシステムの構築を考えています。<br>
なので関連するピサグシステムやOCRシステムの開発を考えました。<br>
(ピサグはレガシーシステムの多い日本に必要な技術・システム、<br>
OCRはPDFファイルなどの価値ある情報や歴史的技術などの活用を考えて途中ですが開発を考えました。)<br><br>

・出品する際にAIによる管理システムの向上や、<br>
購入後から出品完了までのシステムを自動フルフィルメントなどや、<br>
配達員と購入カスタマーを繋ぐモバイルアプリ管理システムも考えています。<br><br>




# 次のステップ提案<br>
・
<br><br><br>
<h2>この先の内容の・開発使用技術(言語・フレームワーク等) 以外は模擬案件と同じ内容です。</h2>
<br><br><br>




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

