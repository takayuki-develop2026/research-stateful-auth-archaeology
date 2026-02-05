# アプリケーション名： (OmniCommerceCore)Stateful〜AWS+AuthO認証、DDD、マルチテナント、AI、 決済マルチゲートウェイ、システム<br>

リポジトリ名：<br>
research-stateful-auth-archaeology<br>
ブランチ名：<br>
- main<br>

それぞれのブランチのREADMEを参照してセットアップ<br>

# 環境構築
Dockerビルド
<br>
<br>
　1\. 　git cloneリンク（ターミナルコマンド） git clone https://github.com/takayuki2025/research-stateful-auth-archaeology.git  の実行

<br>
 　2\.   　ダミーデーターの商品画像ファイルをstrageディレクトリーの中にitem_imagesディレクトリーを作成して商品画像ファイルをコピーする。<br>
　　　（ターミナルコマンド）cd backend (実行後) mkdir storage/app/public/item_images　の実行<br>
　　　　　　　　　　　cp -r public/pictures/* storage/app/public/item_images　の実行<br>
 　3\.　　ダミーデーターのユーザー初期画像ファイルをstrageディレクトリーの中にimagesディレクトリーを作成して初期画像ファイルをコピーする<br>
 　　　（ターミナルコマンド）mkdir storage/app/public/images　の実行<br>
　　　　　　　　　　　cp -r public/pictures_user/* storage/app/public/images　の実行<br>
　4\. Docker Desktopを立ち上げて（ターミナルコマンド）docker-compose up -d --build　の実行
<br>
　
  <br>
laravel環境構築
<br>
<br>
　1\. （ターミナルコマンド）docker-compose exec php bash　の実行
<br>
　2\. （PHPコンテナー）composer install　の実行
<br>
　3\. 　env.exampleファイルから.envを作成し、.envファイルの環境変数を変更<br>
　(PHPコンテナー)  cp .env.example .env　の実行後.envの環境変数の変更<br>

　・stripeの公開キーなどは個人情報保護のためgitで追跡していません。必要でしたらコード伝えます。<br>

  ・firebaseのAPIキーなど６項目（frontend .env　追記用）、firebaseのサービスアカウントキー（新規ファイル作成用）などは個人情報保護のためgitで追跡していません。（画面表示、新規登録もログインもできない状態です。）
  必要でしたら　frontend .env　追記用　と　./backend/config/firebase-service-account.json　ファイルに必要なコード伝えます。<br>

  (.env修正後反映させるため　カレントディレクトリーに戻り)　docker compose restart　の実行   <br>

<br>
<br>
　4\. アプリケーションキーの作成<br>
　　（PHPコンテナー）php artisan key:generate
<br>
　5\. マイグレーションの実行・シーディング実行<br>
　　php artisan migrate:fresh --seed
<br>
　6\.(API ベース開発に変更したため全てのテストコードは使えない状態です。) テスト用のデーターベース作成からPHPUnitテスト実行まで。<br>
　（exitでターミナルに戻ってから）docker-compose exec mysql bash　を実行<br>
　（mysqlコンテナー）mysql -u root -p   の実行後パスワード　root　と入力して実行<br>
　（mysql接続後）CREATE DATABASE coachtech1_test;　を実行 (実行後exitコマンドでターミナルまで戻る)<br>
（ターミナルで　docker-compose exec php bash を実行した後のPHPコンテナーで）php artisan test　を実行してテストをしてください。<br>
<br><br>

-  ダミーのユーザーデーターと出品商品データーのシーダーファイルで作りましたので、PHPコンテナーで上記の通り　php artisan db:seed　を実行してください。<br>
   ダミーのユーザー情報です。'　'は削除してください。<br>
   １：名前:'テスト用のユーザ１'、アドレス:　'valid.email@example.com'　パスワード:　'Testtest1'　出品数：'２品'　ロール：Shop Owner（Shop Ownerはそれぞれのショップのダッシュボードにログイン後移動します。）<br>
   ２：名前:'テスト用のユーザ2'、アドレス:　'taro.y@coachtech.com'　パスワード:　'Testtest2'　出品数：'２品'　ロール：Shop Owner<br>
   ３：名前:'テスト用のユーザ3'、アドレス:　'reina.n@coachtech.com'　パスワード:　'Testtest3'　出品数：'３品'　ロール：Shop Owner<br>
   ４：名前:'テスト用のユーザ4'、アドレス:　'tomomi.a@coachtech.com'　パスワード:　'Testtest4'　出品数：'３品'　ロール：Shop Owner　です。<br><br>


ストライプ決済実行時<br>
stripe listen --forward-to http://localhost/api/webhooks/stripe (ターミナルで実行のまま)<br>
カード番号：4242 4242 4242 4242<br>
有効期限（未来）・シークレットナンバー・名前、は決まりなし。<br><br>

アディエン決済実行時<br>
ngrok http 80  (ターミナルで実行のまま)<br>
カード番号：4111 1111 1111 1111 /シークレットナンバー：737<br>
有効期限（未来）・名前、は決まりなし。<br><br>

出品時解析システム (ターミナルで実行のまま)<br>
docker compose exec php php artisan queue:work<br><br>


# 次のステップ提案<br>





# 伝えること<br>
-  （応用）のstripe決済機能、メール認証機能、PHPUnitでのテストファイルの作成はできています。<br>stripe決済のクレジットカード番号は、4242 4242 4242 4242　で有効期限日は未来の日にち、セキュリティー番号とメールアドレス(メール形式で)、名前はなんでも大丈夫です。<br>
-  stripe決済の都合上最低決済金額が50円なので少し余裕を持たせて出品商品の最低金額を100円以上(変更)にして設定しました。（バリデーション、テスト含む）<br>
-  カード支払いで商品購入処理完了後に登録したstripeのdashboardを参照すれば処理が成功しているのが分かります。必要があれば伝えます。<br><br>
-  COACHTECHのロゴをクリックするとトップページに、ログインユーザーが商品詳細画面で自分が出品した商品の購入手続きをクリックするとプロフィールページに、ゲストユーザーが購入手続きへ・ヘッダーのマイページ・出品・コメントを送信するをクリックするとログインページに移動するようになっています。<br><br>
-  いいね機能はゲストユーザー、ログインユーザーの自分の出品した商品にはできないようになっています。<br><br>
-  コメント機能はログインユーザーが商品を見てコメントする時と、出品者が出品後に追加でコメントした日時がわかるようにしました。<br><br>
-  PHPUnitのテストファイルはスプレットシートのテストケース一覧のID番号に沿ってtests/Featureディレクトリーに保存してあります。上記に記したテスト用のデーターベースを作成した後phpコンテナーで php artisan test を実行してテストをしてください。 <br><br>
-  Route,Controllerは基本設計書に沿ってファイルの中に基本並び替えしています。<br><br>
-  ダミーのユーザーデーターと出品商品データーのシーダーファイルで作りましたので、PHPコンテナーで上記の通り　php artisan db:seed　を実行してください。<br>
   ダミーのユーザー情報です。'　'は削除してください。<br>
   １：名前:'テスト用のユーザ１'、アドレス:　'valid.email@example.com'　パスワード:　'testtest1'　出品数：'２品'<br>
   ２：名前:'テスト用のユーザ2'、アドレス:　'taro.y@coachtech.com'　パスワード:　'testtest2'　出品数：'２品'<br>
   ３：名前:'テスト用のユーザ3'、アドレス:　'reina.n@coachtech.com'　パスワード:　'testtest3'　出品数：'３品'<br>
   ４：名前:'テスト用のユーザ4'、アドレス:　'tomomi.a@coachtech.com'　パスワード:　'testtest4'　出品数：'３品'　　です。メール認証は登録済みでログイン後トップページに移動します。<br><br>
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
<img width="1920" height="1080" alt="Image" src="https://github.com/user-attachments/assets/77cc71be-eb73-4a46-9a28-6dada2c46e4b" />

# 使用技術<br>
  - PHP: 8.4
  - Laravel: 11.31
  - MySql: 8.3
  - nginx: 1.21.1
  - firebase: 10.12.2
  - React: 19.0.0
  - Next.js: 16.0.0
  - node: 22-bullseye
<br>

# URL<br>
  - フリマアプリトップページ： http://localhost/
  - ユーザー登録： http://localhost/register
  - phpMyAdmin:http://localhost:8080/index.php
  - meilhog： http://localhost:8025/

