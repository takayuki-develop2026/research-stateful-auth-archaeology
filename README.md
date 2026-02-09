# アプリケーション名： (OmniCommerceCore)<br>
# Stateful〜AWS+AuthO認証、DDD、マルチテナント、AI、 決済マルチゲートウェイ、（自動フルフィルメントなども計画）システム<br>

リポジトリ名: - research-stateful-auth-archaeology<br>
ブランチ名:　 - main<br>

それぞれのブランチのREADMEを参照してセットアップ<br>

# 環境構築
<br>
Dockerビルド
<br>
<br>
　1\. git cloneリンク（ターミナルコマンド）<br>
 git clone https://github.com/takayuki-develop2026/research-stateful-auth-archaeology.git  の実行<br>

　2\. cd research-stateful-auth-archaeology  の実行<br><br>

　3\. ダミーデーターの商品画像ファイルをstrageディレクトリーの中にitem_imagesディレクトリーを作成して商品画像ファイルをコピーする。<br>
　　（ターミナルコマンド）cd backend (実行後) mkdir storage/app/public/item_images　の実行<br>
　　　　　　　　　　　cp -r public/pictures/* storage/app/public/item_images　の実行<br>
　4\. ダミーデーターのユーザー初期画像ファイルをstrageディレクトリーの中にimagesディレクトリーを作成して初期画像ファイルをコピーする<br>
　（ターミナルコマンド）mkdir storage/app/public/images　の実行<br>
　　　　　　　　　　　cp -r public/pictures_user/* storage/app/public/images　の実行<br><br>

　5\. env.exampleファイルから.envを作成し、.envファイルの環境変数を変更(backend+frontend+admin_rails)<br>
　　a:(backendディレクトリで実行) cp .env.example .env　の実行後.envの環境変数の変更<br>
(DB_PASSWORD="",と<br>
AUTH0_MANAGEMENT_CLIENT_SECRET="",<br>
(新規登録時403エラーになるのはメール認証完了してないとログインできない仕様だからです。)<br>)
　　b:(frontendディレクトリで実行) cp .env.example .env　の実行<br>
　　c:(admin_railsディレクトリで実行) cp .env.example .env 2>/dev/null || true && test -f .env || touch .env<br>
　の実行<br>

　 シークレットキーなどは個人情報保護のためgitで追跡していません。必要でしたら伝えます。<br>
(backend)<br>
FIREBASE_CREDENTIALS=(現段階では使っていない。jsonファイルなし位置表示のみ),<br>
JWT_SECRET=,DB_PASSWORD=,<br>
AUTH0_MANAGEMENT_CLIENT_SECRET=,<br>
(STRIPE)STRIPE_KEY=,STRIPE_SECRET=,STRIPE_WEBHOOK_SECRET=,<br>
(ADYEN)ADYEN_API_KEY=,ADYEN_HMAC_KEY=,は空です。(////)を削除して各準備お願いします。<br>
  必要でしたら　backend .env　追記用　と　./backend/config/firebase-service-account.json　ファイルに必要なコード伝えます。<br><br>

　6\. Docker Desktopを立ち上げて
（カレントディレクトリー）docker-compose up -d --build　の実行
<br><br>

laravel環境構築
<br>
<br>
　1\. (カレントディレクトリー)docker-compose exec php bash　の実行
<br>
　2\. （PHPコンテナー）composer install　の実行
<br>
　3\. アプリケーションキーの作成<br>
　　（PHPコンテナー）php artisan key:generate
<br>
　4\. マイグレーションの実行・シーディング実行<br>
　　（PHPコンテナー）php artisan migrate:fresh --seed
<br>
　5\. シンボリックリンクの作成<br>
　　（PHPコンテナー）php artisan storage:link
<br>
　6\. フロントエンドのセットアップ。<br>
　　 (frontendディレクトリー)npm i　の実行
<br><br>

-  シーダーファイルでユーザーデーターと出品商品データーを作成しました。<br>
   ユーザー情報です。メールの'　'は削除してください。<br>
   １：名前:'テスト用のユーザ１'、アドレス:　'valid.email@example.com'　パスワード:　'Testtest1'　出品数：'２品'<br>
   ロール：Shop Owner（各Shop Owner(manager、staff)は<br>
   ログイン後それぞれのショップのダッシュボードに移動します。）<br>
   (各 テストショップのトップページから管理画面ボタンで各ショップのダッシュボードに入れます。)<br>
   ２：名前:'テスト用のユーザ2'、アドレス:　'taro.y@coachtech.com'　パスワード:　'Testtest2'　出品数：'２品'　ロール：Shop Owner<br>
   ３：名前:'テスト用のユーザ3'、アドレス:　'reina.n@coachtech.com'　パスワード:　'Testtest3'　出品数：'３品'　ロール：Shop Owner<br>
   ４：名前:'テスト用のユーザ4'、アドレス:　'tomomi.a@coachtech.com'　パスワード:　'Testtest4'　出品数：'３品'　ロール：Shop Owner　です。<br><br>

- Stripe決済実行前<br>
（ターミナルコマンド）stripe listen --forward-to http://localhost/api/webhooks/stripe (ターミナルで実行のまま)<br>
カード番号：4242 4242 4242 4242<br>
有効期限（未来）・シークレットナンバー・名前、は決まりなし。<br>
コンビニ払いは現在Stripeのみで決済後3分ほどでダッシュボードに反映<br><br>

- Adyen決済実行前<br>
（ターミナルコマンド）ngrok http 80  (ターミナルで実行のまま)<br>
カード番号：4111 1111 1111 1111 /シークレットナンバー：737<br>
有効期限（未来）・名前、は決まりなし。<br><br>

- AI解析システム（出品解析システム：出品する前に実行） <br>
（ターミナルコマンド）docker compose exec php php artisan queue:work(ターミナルで実行のまま)<br><br>


# アプリの仕様計画<br>
・Adminでショップ運営の権限を与えることができて(ShopOwner付与)<br>
ShopOwnerからManageとStaffの権限を与えることができる。(個人も申請すれば出店できる)<br>
・出品の際マークが付いているのは中古商品としてのマーク<br>
個人出品の場合は💫、ショップの中古商品は⭐️となる<br>


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

