package chatauto



import "testing"



func TestParseRoteiroResponse(t *testing.T) {

	raw := `{"autodom":{"humor":"tesao","posicao":"oral","intensidade":6,"velocidade":7}}`

	roteiro, err := ParseRoteiroResponse(raw)

	if err != nil {

		t.Fatalf("ParseRoteiroResponse: %v", err)

	}

	if roteiro.Posicao != PoseOral || roteiro.Intensidade != 6 || roteiro.Velocidade != 7 {

		t.Fatalf("roteiro = %+v", roteiro)

	}

}



func TestParseReplyResponse(t *testing.T) {

	reply, err := ParseReplyResponse(`{"reply":"Vem mais perto."}`)

	if err != nil {

		t.Fatalf("ParseReplyResponse: %v", err)

	}

	if reply != "Vem mais perto." {

		t.Fatalf("reply = %q", reply)

	}

}

