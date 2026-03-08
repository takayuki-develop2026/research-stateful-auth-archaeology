package com.trustledger.paymentcore.admin.api;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.util.StringUtils;
import org.springframework.web.bind.annotation.*;

import java.time.Instant;
import java.util.*;

@RestController
@RequestMapping("/api/admin/trustledger")
public class AdminTrustLedgerController {

  @Value("${trustledger.adminApiKey:}")
  private String adminApiKey;

  private ResponseEntity<Map<String, Object>> unauthorized() {
    Map<String, Object> body = new LinkedHashMap<>();
    body.put("message", "unauthorized");
    body.put("status", 401);
    body.put("time", Instant.now().toString());
    return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(body);
  }

  private boolean isAuthorized(String key) {
    if (!StringUtils.hasText(adminApiKey)) {
      return true;
    }
    return StringUtils.hasText(key) && adminApiKey.equals(key);
  }

  @GetMapping("/health")
  public ResponseEntity<Map<String, Object>> health(
      @RequestHeader(value = "X-Admin-Key", required = false) String key
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("status", "ok");
    body.put("service", "payment_core_admin");
    body.put("time", Instant.now().toString());
    body.put("path", "/api/admin/trustledger/health");

    return ResponseEntity.ok(body);
  }

  @GetMapping("/webhooks/events")
  public ResponseEntity<Map<String, Object>> listWebhookEvents(
      @RequestHeader(value = "X-Admin-Key", required = false) String key,
      @RequestParam(value = "status", required = false) String status,
      @RequestParam(value = "limit", required = false) Integer limit,
      @RequestParam(value = "days", required = false) Integer days
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("items", Collections.emptyList());
    body.put("count", 0);
    body.put("status_filter", status);
    body.put("limit", limit == null ? 50 : limit);
    body.put("days", days == null ? 7 : days);
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }

  @GetMapping("/kpis/global")
  public ResponseEntity<Map<String, Object>> getGlobalKpis(
      @RequestHeader(value = "X-Admin-Key", required = false) String key,
      @RequestParam(value = "from", required = false) String from,
      @RequestParam(value = "to", required = false) String to
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    String resolvedFrom = (from == null || from.isBlank()) ? "" : from;
    String resolvedTo = (to == null || to.isBlank()) ? "" : to;

    Map<String, Object> adyen = new LinkedHashMap<>();
    adyen.put("sales_total", 0);
    adyen.put("refund_total", 0);
    adyen.put("fee_total", 0);
    adyen.put("net_total", 0);
    adyen.put("postings_count", 0);

    Map<String, Object> byProvider = new LinkedHashMap<>();
    byProvider.put("adyen", adyen);

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("from", resolvedFrom);
    body.put("to", resolvedTo);
    body.put("currency", "JPY");

    // Rails view が期待しているキー
    body.put("sales_total", 0);
    body.put("refund_total", 0);
    body.put("fee_total", 0);
    body.put("net_total", 0);
    body.put("postings_count", 0);
    body.put("by_provider", byProvider);

    // 監査/将来拡張用
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }

  @GetMapping("/kpis/shops")
  public ResponseEntity<Map<String, Object>> getShopKpis(
      @RequestHeader(value = "X-Admin-Key", required = false) String key,
      @RequestParam(value = "from", required = false) String from,
      @RequestParam(value = "to", required = false) String to,
      @RequestParam(value = "limit", required = false) Integer limit
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    String resolvedFrom = (from == null || from.isBlank()) ? "" : from;
    String resolvedTo = (to == null || to.isBlank()) ? "" : to;

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("from", resolvedFrom);
    body.put("to", resolvedTo);
    body.put("currency", "JPY");
    body.put("items", Collections.emptyList());
    body.put("count", 0);
    body.put("limit", limit == null ? 20 : limit);
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }

  @GetMapping("/postings")
  public ResponseEntity<Map<String, Object>> searchPostings(
      @RequestHeader(value = "X-Admin-Key", required = false) String key
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("items", Collections.emptyList());
    body.put("count", 0);
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }

  @GetMapping("/reconciliation/missing-sales")
  public ResponseEntity<Map<String, Object>> listMissingSales(
      @RequestHeader(value = "X-Admin-Key", required = false) String key
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("items", Collections.emptyList());
    body.put("count", 0);
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }

  @GetMapping("/shops")
  public ResponseEntity<Map<String, Object>> listShops(
      @RequestHeader(value = "X-Admin-Key", required = false) String key
  ) {
    if (!isAuthorized(key)) {
      return unauthorized();
    }

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("items", Collections.emptyList());
    body.put("count", 0);
    body.put("time", Instant.now().toString());

    return ResponseEntity.ok(body);
  }
}