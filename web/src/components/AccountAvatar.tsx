import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { UserAccount } from "../api/types";

export function accountMonogram(username: string): string {
  return (Array.from(username.trim())[0] ?? "?").toUpperCase();
}

export function AccountAvatar({ account, className = "" }: { account: UserAccount; className?: string }) {
  const image = api.accountProfileImageURL(account);
  const [imageFailed, setImageFailed] = useState(false);
  useEffect(() => setImageFailed(false), [image]);
  return (
    <span className={`account-avatar ${className}`.trim()} aria-hidden="true">
      {image && !imageFailed
        ? <img src={image} alt="" decoding="async" onError={() => setImageFailed(true)} />
        : <span>{accountMonogram(account.username)}</span>}
    </span>
  );
}
