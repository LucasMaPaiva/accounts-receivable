-- Origem do título: a venda do monólito. Fica NULL quando o título é criado
-- direto pela API. Volta no evento titulo.pago para o monólito dar baixa na venda.
ALTER TABLE titulos ADD COLUMN venda_id text;
