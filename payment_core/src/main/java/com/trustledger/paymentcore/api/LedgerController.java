package com.trustledger.paymentcore.api;

import com.trustledger.paymentcore.api.dto.PostLedgerCommandDto;
import com.trustledger.paymentcore.api.dto.PostLedgerResponseDto;
import jakarta.validation.Valid;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.util.StringUtils;
import org.springframework.web.bind.annotation.*;

import java.time.Instant;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

@RestController
@RequestMapping("/ledger")
public class LedgerController {
  private static final Logger log = LoggerFactory.getLogger(LedgerController.class);

  // v0: インメモリ冪等（次でMySQL/Redisに置換）
  private final Set<String> processed = ConcurrentHashMap.newKeySet();

  @Value("${trustledger.apiKey:}")
  private String apiKey;

  @PostMapping("/post")
  public ResponseEntity<PostLedgerResponseDto> post(
      @RequestHeader(value = "X-TrustLedger-Key", required = false) String key,
      @Valid @RequestBody PostLedgerCommandDto cmd
  ) {
    // --- 最小認証（設定されているときだけ必須） ---
    if (StringUtils.hasText(apiKey)) {
      if (!StringUtils.hasText(key) || !apiKey.equals(key)) {
        return ResponseEntity.status(HttpStatus.UNAUTHORIZED)
            .body(new PostLedgerResponseDto(false, false));
      }
    }

    // --- 冪等キー（source_event_id が正） ---
    boolean first = processed.add(cmd.source_event_id());
    if (!first) {
      log.info("[ledger.post] DUPLICATE ignored: {}", cmd.source_event_id());
      return ResponseEntity.ok(new PostLedgerResponseDto(true, true));
    }

    // --- v0: ここでDBに ledger_postings / ledger_entries を書くのが本体 ---
    // いまは「受け口が動く」ことを確認するためログのみ
    log.info("[ledger.post] accepted: provider={} event={} type={} amount={} {} shop={} order={} payment={} replay={}",
        cmd.source_provider(), cmd.source_event_id(), cmd.posting_type(), cmd.amount(), cmd.currency(),
        cmd.shop_id(), cmd.order_id(), cmd.payment_id(), cmd.replay()
    );

    return ResponseEntity.ok(new PostLedgerResponseDto(true, false));
  }

  @GetMapping("/health")
  public ResponseEntity<Map<String, Object>> health() {
    Map<String, Object> body = new HashMap<>();
    body.put("status", "ok");
    body.put("service", "payment_core");
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }
}