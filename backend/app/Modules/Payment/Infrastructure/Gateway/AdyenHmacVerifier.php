<?php

declare(strict_types=1);

namespace App\Modules\Payment\Infrastructure\Gateway;

final class AdyenHmacVerifier
{
    public function __construct(
        private string $hmacKey, // from config('services.adyen.hmac_key')
    ) {
    }

    /**
     * Adyen Standard webhook HMAC verification (JSON notifications)
     * signature is in additionalData.hmacSignature (base64)
     *
     * See Adyen docs: HMAC signature for notifications.
     *
     * @param array<string,mixed> $nri NotificationRequestItem
     */
    public function verify(array $nri): bool
    {
        $additional = $nri['additionalData'] ?? null;
        if (!is_array($additional)) return false;

        $given = $additional['hmacSignature'] ?? null;
        if (!is_string($given) || $given === '') return false;

        $signingString = $this->buildSigningString($nri);

        $keyBin = $this->decodeKeyToBinary($this->hmacKey);
        if ($keyBin === null) return false;

        $mac = hash_hmac('sha256', $signingString, $keyBin, true);
        $expected = base64_encode($mac);

        // timing safe compare
        return hash_equals($expected, $given);
    }

    /**
     * Concatenate these fields with ":" (Adyen spec)
     * pspReference:originalReference:merchantAccountCode:merchantReference:amount.value:amount.currency:eventCode:success
     */
    private function buildSigningString(array $nri): string
    {
        $pspRef = (string)($nri['pspReference'] ?? '');
        $origRef = (string)($nri['originalReference'] ?? '');
        $merchantAccount = (string)($nri['merchantAccountCode'] ?? '');
        $merchantRef = (string)($nri['merchantReference'] ?? '');

        $amount = $nri['amount'] ?? null;
        $amountValue = '';
        $amountCurrency = '';
        if (is_array($amount)) {
            $amountValue = (string)($amount['value'] ?? '');
            $amountCurrency = (string)($amount['currency'] ?? '');
        }

        $eventCode = (string)($nri['eventCode'] ?? '');
        $success = (string)($nri['success'] ?? '');

        return implode(':', [
            $pspRef,
            $origRef,
            $merchantAccount,
            $merchantRef,
            $amountValue,
            $amountCurrency,
            $eventCode,
            $success,
        ]);
    }

    /**
     * Adyen HMAC key is typically hex. Support hex or base64 just in case.
     */
    private function decodeKeyToBinary(string $key): ?string
    {
        $k = trim($key);
        if ($k === '') return null;

        // hex?
        if (preg_match('/\A[0-9a-fA-F]+\z/', $k) === 1 && (strlen($k) % 2 === 0)) {
            $bin = hex2bin($k);
            return $bin === false ? null : $bin;
        }

        // base64?
        $bin = base64_decode($k, true);
        if ($bin !== false) return $bin;

        return null;
    }
}