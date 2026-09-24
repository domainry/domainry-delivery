package presentation

import "strings"

var SupportedLocales = []string{"en", "zh", "zh-hant", "ja", "ko", "es", "pt", "fr", "de", "it", "tr", "ar"}

func NormalizeLocale(value string) string {
	for _, candidate := range strings.Split(value, ",") {
		candidate = strings.TrimSpace(strings.SplitN(candidate, ";", 2)[0])
		candidate = strings.ToLower(strings.ReplaceAll(candidate, "_", "-"))
		if candidate == "zh-hant" || strings.HasPrefix(candidate, "zh-hant-") || candidate == "zh-tw" || candidate == "zh-hk" || candidate == "zh-mo" {
			return "zh-hant"
		}
		candidate = strings.SplitN(candidate, "-", 2)[0]
		for _, supported := range SupportedLocales {
			if candidate == supported {
				return supported
			}
		}
	}
	return "en"
}

func ProjectionForLocale(run DeliveryRun, locale string) Projection {
	projection := ProjectionFor(run)
	locale = NormalizeLocale(locale)
	if locale == "en" {
		return projection
	}
	labels := releaseGateLabels[locale]
	for index := range projection.Workflow.ReleaseGates {
		gate := &projection.Workflow.ReleaseGates[index]
		if label := labels[gate.Code]; label != "" {
			gate.Label = label
		}
	}
	return projection
}

func LocalizeError(err *Error, locale string) *Error {
	if err == nil || NormalizeLocale(locale) == "en" {
		return err
	}
	locale = NormalizeLocale(locale)
	kind := "validation"
	switch err.Code {
	case "authentication_required":
		kind = "authentication"
	case "permission_denied", "identity_workspace_mismatch", "actor_unauthorized":
		kind = "permission"
	case "not_found":
		kind = "not_found"
	case "revision_conflict", "idempotency_key_reused":
		kind = "conflict"
	case "storage_failure", "internal_error":
		kind = "internal"
	}
	message := errorMessages[locale][kind]
	if message == "" {
		return err
	}
	return &Error{Code: err.Code, Message: message, Details: err.Details}
}

var releaseGateLabels = map[string]map[string]string{
	"zh":      {"feature_frozen": "功能版本已冻结", "development_completed": "开发已完成", "product_revision_bound": "可执行产品版本已绑定", "technical_tests_passed": "技术测试已通过", "issues_closed": "所有问题已关闭", "acceptance_passed": "业务验收已通过", "release_checks_passed": "上线检查已通过"},
	"zh-hant": {"feature_frozen": "功能版本已凍結", "development_completed": "開發已完成", "product_revision_bound": "可執行產品版本已綁定", "technical_tests_passed": "技術測試已通過", "issues_closed": "所有問題已關閉", "acceptance_passed": "業務驗收已通過", "release_checks_passed": "上線檢查已通過"},
	"ja":      {"feature_frozen": "FeatureRevision を固定済み", "development_completed": "開発完了", "product_revision_bound": "実行可能な ProductRevision を関連付け済み", "technical_tests_passed": "技術テスト合格", "issues_closed": "すべての Issue をクローズ", "acceptance_passed": "業務受入合格", "release_checks_passed": "リリースチェック合格"},
	"ko":      {"feature_frozen": "FeatureRevision 고정됨", "development_completed": "개발 완료", "product_revision_bound": "실행 가능한 ProductRevision 연결됨", "technical_tests_passed": "기술 테스트 통과", "issues_closed": "모든 Issue 종료", "acceptance_passed": "비즈니스 승인 통과", "release_checks_passed": "릴리스 점검 통과"},
	"es":      {"feature_frozen": "FeatureRevision fijada", "development_completed": "Desarrollo completado", "product_revision_bound": "ProductRevision ejecutable vinculada", "technical_tests_passed": "Pruebas técnicas superadas", "issues_closed": "Todos los Issues cerrados", "acceptance_passed": "Aceptación de negocio superada", "release_checks_passed": "Comprobaciones de publicación superadas"},
	"pt":      {"feature_frozen": "FeatureRevision fixada", "development_completed": "Desenvolvimento concluído", "product_revision_bound": "ProductRevision executável vinculada", "technical_tests_passed": "Testes técnicos aprovados", "issues_closed": "Todos os Issues fechados", "acceptance_passed": "Aceitação de negócio aprovada", "release_checks_passed": "Verificações de publicação aprovadas"},
	"fr":      {"feature_frozen": "FeatureRevision figée", "development_completed": "Développement terminé", "product_revision_bound": "ProductRevision exécutable liée", "technical_tests_passed": "Tests techniques réussis", "issues_closed": "Tous les Issues fermés", "acceptance_passed": "Acceptation métier réussie", "release_checks_passed": "Contrôles de publication réussis"},
	"de":      {"feature_frozen": "FeatureRevision fixiert", "development_completed": "Entwicklung abgeschlossen", "product_revision_bound": "Ausführbare ProductRevision gebunden", "technical_tests_passed": "Technische Tests bestanden", "issues_closed": "Alle Issues geschlossen", "acceptance_passed": "Fachliche Abnahme bestanden", "release_checks_passed": "Release-Prüfungen bestanden"},
	"it":      {"feature_frozen": "FeatureRevision bloccata", "development_completed": "Sviluppo completato", "product_revision_bound": "ProductRevision eseguibile collegata", "technical_tests_passed": "Test tecnici superati", "issues_closed": "Tutti gli Issue chiusi", "acceptance_passed": "Accettazione business superata", "release_checks_passed": "Controlli di rilascio superati"},
	"tr":      {"feature_frozen": "FeatureRevision sabitlendi", "development_completed": "Geliştirme tamamlandı", "product_revision_bound": "Çalıştırılabilir ProductRevision bağlandı", "technical_tests_passed": "Teknik testler geçti", "issues_closed": "Tüm Issue kayıtları kapalı", "acceptance_passed": "İş kabulü geçti", "release_checks_passed": "Yayın kontrolleri geçti"},
	"ar":      {"feature_frozen": "تم تجميد مراجعة الميزة", "development_completed": "اكتمل التطوير", "product_revision_bound": "تم ربط مراجعة المنتج القابلة للتنفيذ", "technical_tests_passed": "نجحت الاختبارات التقنية", "issues_closed": "تم إغلاق جميع المشكلات", "acceptance_passed": "نجح قبول العمل", "release_checks_passed": "نجحت فحوصات النشر"},
}

var errorMessages = map[string]map[string]string{
	"zh":      {"authentication": "需要经过认证的身份。", "permission": "你没有权限执行此操作。", "not_found": "没有找到请求的资源。", "conflict": "资源已经发生变化，请刷新后重新提交。", "internal": "服务器无法处理本次请求。", "validation": "本次请求不符合当前 Delivery 规则。"},
	"zh-hant": {"authentication": "需要經過驗證的身分。", "permission": "你沒有權限執行此操作。", "not_found": "找不到要求的資源。", "conflict": "資源已經發生變化，請重新整理後再提交。", "internal": "伺服器無法處理本次要求。", "validation": "本次要求不符合目前的 Delivery 規則。"},
	"ja":      {"authentication": "認証済みの Identity が必要です。", "permission": "この操作を実行する権限がありません。", "not_found": "要求されたリソースが見つかりません。", "conflict": "リソースが変更されました。再読み込みしてから再送信してください。", "internal": "サーバーが要求を処理できませんでした。", "validation": "要求は現在の Delivery ルールを満たしていません。"},
	"ko":      {"authentication": "인증된 Identity가 필요합니다.", "permission": "이 작업을 수행할 권한이 없습니다.", "not_found": "요청한 리소스를 찾을 수 없습니다.", "conflict": "리소스가 변경되었습니다. 다시 읽은 후 제출하세요.", "internal": "서버가 요청을 처리할 수 없습니다.", "validation": "요청이 현재 Delivery 규칙을 충족하지 않습니다."},
	"es":      {"authentication": "Se requiere una identidad autenticada.", "permission": "No tienes permiso para realizar esta operación.", "not_found": "No se encontró el recurso solicitado.", "conflict": "El recurso cambió; vuelve a leerlo antes de enviarlo.", "internal": "El servidor no pudo procesar la solicitud.", "validation": "La solicitud no cumple las reglas actuales de Delivery."},
	"pt":      {"authentication": "É necessária uma identidade autenticada.", "permission": "Você não tem permissão para realizar esta operação.", "not_found": "O recurso solicitado não foi encontrado.", "conflict": "O recurso mudou; leia-o novamente antes de enviar.", "internal": "O servidor não conseguiu processar a solicitação.", "validation": "A solicitação não atende às regras atuais do Delivery."},
	"fr":      {"authentication": "Une identité authentifiée est requise.", "permission": "Vous n’êtes pas autorisé à effectuer cette opération.", "not_found": "La ressource demandée est introuvable.", "conflict": "La ressource a changé ; relisez-la avant de soumettre.", "internal": "Le serveur n’a pas pu traiter la demande.", "validation": "La demande ne respecte pas les règles Delivery actuelles."},
	"de":      {"authentication": "Eine authentifizierte Identity ist erforderlich.", "permission": "Sie sind für diesen Vorgang nicht berechtigt.", "not_found": "Die angeforderte Ressource wurde nicht gefunden.", "conflict": "Die Ressource wurde geändert; lesen Sie sie vor dem Absenden erneut.", "internal": "Der Server konnte die Anfrage nicht verarbeiten.", "validation": "Die Anfrage erfüllt die aktuellen Delivery-Regeln nicht."},
	"it":      {"authentication": "È richiesta un’Identity autenticata.", "permission": "Non hai il permesso di eseguire questa operazione.", "not_found": "La risorsa richiesta non è stata trovata.", "conflict": "La risorsa è cambiata; rileggila prima di inviare.", "internal": "Il server non ha potuto elaborare la richiesta.", "validation": "La richiesta non rispetta le regole Delivery correnti."},
	"tr":      {"authentication": "Kimliği doğrulanmış bir Identity gereklidir.", "permission": "Bu işlemi yapma izniniz yok.", "not_found": "İstenen kaynak bulunamadı.", "conflict": "Kaynak değişti; göndermeden önce yeniden okuyun.", "internal": "Sunucu isteği işleyemedi.", "validation": "İstek mevcut Delivery kurallarını karşılamıyor."},
	"ar":      {"authentication": "يلزم وجود هوية موثقة.", "permission": "ليس لديك إذن لتنفيذ هذه العملية.", "not_found": "لم يتم العثور على المورد المطلوب.", "conflict": "تم تغيير المورد؛ حدّثه قبل الإرسال مرة أخرى.", "internal": "تعذر على الخادم معالجة الطلب.", "validation": "لا يطابق الطلب قواعد Delivery الحالية."},
}
